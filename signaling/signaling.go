package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/websocket"
)

type SignalingServer struct {
	users    []*User
	upgrader websocket.Upgrader
	mux      sync.Mutex
}

func (ss *SignalingServer) AddUser(conn *websocket.Conn, name string) {
	ss.mux.Lock()
	defer ss.mux.Unlock()

	ss.users = append(ss.users, &User{Name: name, Conn: conn})
}

func (ss *SignalingServer) AllUserNames() []string {
	ss.mux.Lock()
	defer ss.mux.Unlock()

	users := make([]string, len(ss.users))
	for _, user := range ss.users {
		users = append(users, user.Name)
	}

	return users
}

func (ss *SignalingServer) NotifyUsers(notify func(*User)) {
	// TODO: handle error
	for _, user := range ss.users {
		notify(user)
	}
}

func (ss *SignalingServer) PeerFromConn(conn *websocket.Conn) *websocket.Conn {
	for _, user := range ss.users {
		if user.Conn == conn {
			for _, peerUser := range ss.users {
				if peerUser.Name == user.Peer {
					return peerUser.Conn
				}
			}
		}
	}

	return nil
}

func (ss *SignalingServer) PeerFromName(name string) *User {
	for _, user := range ss.users {
		for _, peerUser := range ss.users {
			if user.Peer == peerUser.Name {
				return peerUser
			}
		}
	}

	return nil
}

func (ss *SignalingServer) UserFromConn(conn *websocket.Conn) *User {
	for _, user := range ss.users {
		if user.Conn == conn {
			return user
		}
	}

	return nil
}

func (ss *SignalingServer) UserFromName(name string) *User {
	for _, user := range ss.users {
		if user.Name == name {
			return user
		}
	}

	return nil
}

func (ss *SignalingServer) UpdatePeer(origin, peer string) error {
	ss.mux.Lock()
	defer ss.mux.Unlock()

	for _, user := range ss.users {
		if user.Name == origin {
			user.Peer = peer
			return nil
		}
	}

	return errors.New("missing origin user")
}

// User template
type User struct {
	Name string
	Peer string
	Conn *websocket.Conn
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

// offerEvent forwards an offer to a remote peer
func (ss *SignalingServer) offerEvent(conn *websocket.Conn, data SignalMessage) error {
	var sm SignalMessage

	author := ss.UserFromConn(conn)
	if author == nil {
		log.Errorf("offerEvent: unregistered author")
		return errors.New("unregistered author")
	}

	peer := ss.UserFromName(data.Name)
	if peer == nil {
		log.Errorf("offerEvent: unknown peer author")
		return errors.New("unknown peer")
	}

	log.Infof("offerEvent: message received from %v", author.Name)

	if err := ss.UpdatePeer(author.Name, peer.Name); err != nil {
		return err
	}

	log.Debugf("offerEvent: set peer of author %v to %v", author.Name, peer.Name)

	sm.Name = author.Name
	sm.Offer = data.Offer
	sm.Type = "offer"
	out, err := json.Marshal(sm)
	if err != nil {
		return err
	}

	if err = peer.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
		return err
	}

	log.Infof("Offer forwarded to %v", peer.Name)

	return nil
}

//answerEvent forwards Answer to original peer
func (ss *SignalingServer) answerEvent(conn *websocket.Conn, data SignalMessage) error {
	var sm SignalMessage

	author := ss.UserFromConn(conn)
	if author == nil {
		return errors.New("unregistered author")
	}

	peer := ss.UserFromName(data.Name)
	if peer == nil {
		return errors.New("unknown requested peer")
	}

	if err := ss.UpdatePeer(author.Name, data.Name); err != nil {
		return err
	}

	sm.Answer = data.Answer
	sm.Type = "answer"
	out, err := json.Marshal(sm)
	if err != nil {
		return err
	}

	if err = peer.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
		return err
	}

	log.Infof("answerEvent: answer forwarded to %v", author.Name)

	return nil
}

//candidateEvent forwards candidate to original peer
func (ss *SignalingServer) candidateEvent(conn *websocket.Conn, data SignalMessage) error {
	var sm SignalMessage

	author := ss.UserFromConn(conn)
	if author == nil {
		return errors.New("unregistered connection")
	}

	log.Infof("candidateEvent received from %v", author.Name)

	peer := ss.UserFromName(data.Name)
	if peer == nil {
		return errors.New("unregistered peer")
	}

	sm.Candidate = data.Candidate
	sm.Type = "candidate"
	out, err := json.Marshal(sm)
	if err != nil {
		return err
	}

	if err = peer.Conn.WriteMessage(websocket.TextMessage, out); err != nil {
		return err
	}

	log.Infof("candidateEvent: candidate forwarded to %v", peer.Name)

	return nil
}

// leaveEvent terminates a connection (client closed the browser for example)
func (ss *SignalingServer) leaveEvent(conn *websocket.Conn) error {
	defer conn.Close()

	log.Info("received terminated connection request")

	if peerConn := ss.PeerFromConn(conn); peerConn != nil {
		var out []byte
		out, err := json.Marshal(Leaving{Type: "leaving"})
		if err != nil {
			return err
		}

		err = peerConn.WriteMessage(websocket.TextMessage, out)
		if err != nil {
			return err
		}
	}

	return nil
}

// loginEvent
func (ss *SignalingServer) loginEvent(conn *websocket.Conn, data SignalMessage) error {
	if author := ss.UserFromName(data.Name); author != nil {
		log.Debugf("loginEvent: %v tried to log in but was already registered: %v", data.Name, ss.AllUserNames())
		return nil
	}

	ss.AddUser(conn, data.Name)
	out, err := json.Marshal(LoginResponse{Type: "login", Success: true})
	if err != nil {
		return err
	}

	log.Infof("User %v registered in successfully", data.Name)

	if err = conn.WriteMessage(websocket.TextMessage, out); err != nil {
		return err
	}

	out, err = json.Marshal(Users{Type: "users", Users: ss.AllUserNames()})
	if err != nil {
		return err
	}

	ss.NotifyUsers(func(user *User) {
		user.Conn.WriteMessage(websocket.TextMessage, out)
	})

	return nil
}

// unknownCommandEvent
func unknownCommandEvent(conn *websocket.Conn, raw []byte) error {
	var out []byte
	var message SignalMessage

	err := json.Unmarshal(raw, &message)
	if err != nil {
		return err
	}

	log.Infof("unrecognized command %v", string(raw))

	out, err = json.Marshal(DefaultError{Type: "error", Message: "Unrecognized command"})
	if err != nil {
		return err
	}

	if err = conn.WriteMessage(websocket.TextMessage, out); err != nil {
		return err
	}

	return nil
}

func (ss *SignalingServer) connHandler(conn *websocket.Conn) error {
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
		err = ss.loginEvent(conn, message)
	case "offer":
		err = ss.offerEvent(conn, message)
	case "answer":
		err = ss.answerEvent(conn, message)
	case "candidate":
		err = ss.candidateEvent(conn, message)
	case "leave":
		err = ss.leaveEvent(conn)
	default:
		err = unknownCommandEvent(conn, raw)
	}

	if err != nil {
		log.Errorf("[%vEvent]: %v", message.Type, err)
		return err
	}

	return nil
}

func (ss *SignalingServer) Handler(w http.ResponseWriter, r *http.Request) {
	conn, err := ss.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	log.Debugf("%v accesses the server", conn.RemoteAddr())
	for {
		if err := ss.connHandler(conn); err != nil {
			if err.Error() == "websocket: close 1001 (going away)" {
				user := ss.UserFromConn(conn)
				if user == nil {
					log.Debugf("connection closed for %v", conn.RemoteAddr())
				} else {
					ss.leaveEvent(conn)
					log.Debugf("connection closed for %v", user.Name)
				}
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
	// Ugrade policty from http request to websocket
	// TODO: to be defined
	ss := SignalingServer{
		users: []*User{},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	http.HandleFunc("/", ss.Handler)

	log.Info("Signaling Server started")

	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Panic(err)
	}
}
