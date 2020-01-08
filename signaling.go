package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/websocket"
)

// USERS is a map of registered users and known connections
var USERS map[string]User

// CONNECTIONS is a map of websocket connections to author
var CONNECTIONS map[*websocket.Conn]string

// User template
type User struct {
	Name string
	Peer string
	Conn *websocket.Conn
}

//Define upgrade policy.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

// SignalMessage template to establish connection
type SignalMessage struct {
	Type      string     `json:"type,omitempty"`
	Name      string     `json:"name,omitempty"`
	Offer     *Offer     `json:"offer,omitempty"`
	Answer    *Answer    `json:"answer,omitempty"`
	Candidate *Candidate `json:"candidate,omitempty"`
}

// LoginResponse is a LoginRequest response from the server
// External struct to manage Success bool independently
type LoginResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
}

//Users list
type Users struct {
	Type  string   `json:"type"`
	Users []string `json:"users"`
}

// Offer struct
type Offer struct {
	Type string `json:"type"`
	Sdp  string `json:"sdp"`
}

// Answer struct
type Answer struct {
	Type string `json:"type"`
	Sdp  string `json:"sdp"`
}

// Candidate struct
type Candidate struct {
	Candidate     string `json:"candidate"`
	SdpMid        string `json:"sdpMid"`
	SdpMLineIndex int    `json:"sdpMLineIndex"`
}

// Leaving struct
type Leaving struct {
	Type string `json:"type"`
}

// DefaultError struct
type DefaultError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Ugrade policty from http request to websocket
// TODO: to be defined
func checkOrigin(r *http.Request) bool {
	//For example: Check in a blacklist if the address is present
	//if blacklist_check(r.RemoteAddr) { return false }
	return true
}

// onOffer forwards an offer to a remote peer
func onOffer(data SignalMessage, conn *websocket.Conn) error {
	var sm SignalMessage

	sm.Offer = data.Offer
	sm.Type = "offer"

	log.Infof("Offer received from %v", CONNECTIONS[conn])

	if peer, isRegistered := USERS[data.Name]; isRegistered {
		sm.Name = CONNECTIONS[conn]
		if _, isAuthorRegistered := USERS[sm.Name]; isAuthorRegistered {
			//map fields are not addressable
			user := USERS[sm.Name]
			user.Peer = data.Name
			USERS[sm.Name] = user
			log.Debugf("set peer of user %v to %v", USERS[sm.Name], data.Name)
		} else {
			return errors.New("offer received by unregistered user")
		}

		out, err := json.Marshal(sm)
		if err != nil {
			return err
		}

		if err = peer.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
			return err
		}

		log.Infof("Offer forwarded to %v", peer.Name)

	} else {
		return errors.New("unregistered peer")
	}

	return nil
}

//onAnswer forwards Answer to original peer
func onAnswer(data SignalMessage, conn *websocket.Conn) (err error) {
	var sm SignalMessage

	sm.Answer = data.Answer
	sm.Type = "answer"

	log.Infof("Answer received from %v", CONNECTIONS[conn])

	if peer, isRegistered := USERS[data.Name]; isRegistered {
		if author, isAuthorRegistered := USERS[CONNECTIONS[conn]]; isAuthorRegistered {
			//Currently, map members are not addressable. Work around here.
			author.Peer = peer.Name
			USERS[CONNECTIONS[conn]] = author
			log.Infof("set peer of user %v to %v", USERS[CONNECTIONS[conn]].Name, peer.Name)
		} else {
			return errors.New("unregistered user")
		}

		out, err := json.Marshal(sm)
		if err != nil {
			return err
		}

		if err = peer.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
			return err
		}

		log.Infof("onAnswer: answer forwarded to %v", peer.Name)
	} else {
		return errors.New("unregistered peer")
	}

	return nil
}

//onCandidate forwards candidate to original peer
func onCandidate(data SignalMessage, conn *websocket.Conn) error {
	var sm SignalMessage

	sm.Candidate = data.Candidate
	sm.Type = "candidate"

	log.Infof("onCandidate received from %v", CONNECTIONS[conn])

	if peer, isRegistered := USERS[data.Name]; isRegistered {
		out, err := json.Marshal(sm)
		if err != nil {
			return err
		}
		if err = peer.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
			return err
		}

		log.Infof("onCandidate: candidate forwarded to %v", peer.Name)

	} else {
		return errors.New("unregistered peer")
	}

	return nil
}

//onLeave forwards leave message to remote Peer and closes the current connection
func onLeave(data SignalMessage, conn *websocket.Conn) error {
	var out []byte
	defer conn.Close()

	log.Infof("onLeave: leave message received from %v", CONNECTIONS[conn])

	out, err := json.Marshal(Leaving{Type: "leaving"})
	if err != nil {
		return err
	}

	user := CONNECTIONS[conn]
	if peer, isRegistered := USERS[USERS[user].Peer]; isRegistered {
		if err = peer.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
			return err
		}
		log.Infof("onLeave: leave message sent to %v", peer.Name)
	}

	return nil
}

// forceLeave terminates a connection (client closed the browser for example)
func forceLeave(conn *websocket.Conn) error {
	var out []byte
	defer conn.Close()

	out, err := json.Marshal(Leaving{Type: "leaving"})
	if err != nil {
		return err
	}

	username := CONNECTIONS[conn]
	if peer, isRegistered := USERS[USERS[username].Peer]; isRegistered {
		if err := peer.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
			return err
		}
		log.Infof("forceLeave: force leaving message sent to %v", peer.Name)
	}
	return nil
}

// onLogin
func onLogin(data SignalMessage, conn *websocket.Conn) error {
	if _, isRegistered := USERS[data.Name]; isRegistered {
		_, err := json.Marshal(LoginResponse{Type: "login", Success: false})
		if err != nil {
			return err
		}

		log.Infof("user %v tried to log in but was denied, %v already registered", conn.RemoteAddr(),
			USERS[data.Name].Name)
	} else {
		USERS[data.Name] = User{Name: data.Name, Conn: conn}
		CONNECTIONS[conn] = data.Name
		out, err := json.Marshal(LoginResponse{Type: "login", Success: true})

		log.Infof("User %v logged in successfully", CONNECTIONS[conn])
		if err != nil {
			return err
		}

		if err = conn.WriteMessage(websocket.TextMessage, out); err != nil {
			return err
		}

		// Notifies all the connections of the new list of users
		// First generate an array of users then send it to each connected user
		var users = make([]string, len(CONNECTIONS))
		for e := range CONNECTIONS {
			users = append(users, CONNECTIONS[e])
		}
		out, err = json.Marshal(Users{Type: "users", Users: users})
		if err != nil {
			return err
		}
		for conn := range CONNECTIONS {
			err := conn.WriteMessage(websocket.TextMessage, out)
			if err != nil {
				log.Error(err)
			}
		}
	}

	return nil
}

// onDefault
func onDefault(raw []byte, conn *websocket.Conn) error {
	var out []byte
	var message SignalMessage

	err := json.Unmarshal(raw, &message)
	if err != nil {
		return err
	}

	if message.Type != "" {
		log.Errorf("onDefault: unrecognized type command found:", message.Type)
	} else {
		log.Errorf("onDefault: unrecognized type command found: %v", raw)
	}

	out, err = json.Marshal(DefaultError{Type: "error", Message: "Unrecognized command"})
	if err != nil {
		return err
	}

	if err = conn.WriteMessage(websocket.TextMessage, out); err != nil {
		return err
	}

	return nil
}

func connHandler(conn *websocket.Conn) error {
	var message SignalMessage

	_, raw, err := conn.ReadMessage()
	if err != nil {
		log.Errorf("connHandler.ReadMessage: %v", err)
		return err
	}

	if err := json.Unmarshal(raw, &message); err != nil {
		log.Errorf("connHandler.Unmarshal: %v", err)

		out, err := json.Marshal(DefaultError{Type: "error", Message: "Incorrect data format"})
		if err != nil {
			log.Errorf("connHandler.Marshal: %v", err)
			return err
		}

		if err = conn.WriteMessage(websocket.TextMessage, out); err != nil {
			log.Errorf("connHandler.WriteMessage: %v", err)
			return err
		}

		return err
	}

	switch message.Type {
	case "login":
		return onLogin(message, conn)
	case "offer":
		return onOffer(message, conn)
	case "answer":
		return onAnswer(message, conn)
	case "candidate":
		return onCandidate(message, conn)
	case "leave":
		return onLeave(message, conn)
	default:
		return onDefault(raw, conn)
	}
}

//reqHandler catches HTTP Requests, upgrade them if needed and let connHandler managing the connection
func reqHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	log.Debugf("%v accesses the server", conn.RemoteAddr())
	for {
		if err := connHandler(conn); err != nil {
			if err.Error() == "websocket: close 1001 (going away)" {
				if user, isRegistered := CONNECTIONS[conn]; isRegistered {
					log.Debugf("connection closed for %v", user)
					forceLeave(conn)
				} else {
					log.Debugf("connection closed for %v", conn.RemoteAddr())
				}
				return
			}
			log.Error(err)
		}
	}
}

func init() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
}

func main() {
	USERS = make(map[string]User)
	CONNECTIONS = make(map[*websocket.Conn]string)
	http.HandleFunc("/", reqHandler)

	log.Info("Signaling Server started")

	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Panic(err)
	}
}
