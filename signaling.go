package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
)

//Hashmap of registered users, in ram
var USERS map[string]User

//Define upgrade policy.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

// Template of a User
type User struct {
	Name string
	Peer string
	Conn *websocket.Conn
}

// Template of input message readable by the server
type MessageInput struct {
	Type      string `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
	Offer     *Offer `json:"offer,omitempty"`
	Answer    string `json:"answer,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// Define Login request sent back from the server
type LoginResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
}

type Offer struct {
	Type string `json:"type"`
	Sdp  string `json:"sdp"`
}

//Ugrade policty from http request to websocket, to be defined
func checkOrigin(r *http.Request) bool {
	//For example: Check in a blacklist if the address is present
	//if blacklist_check(r.RemoteAddr) { return false }
	return true
}

func onOffer(data MessageInput, messageType int, conn *websocket.Conn) error {
	fmt.Println("Offer received")
	var err error
	var out []byte

	if peer, isRegistered := USERS[data.Name]; isRegistered {
		out, err = json.Marshal(data.Offer)
		if err != nil {
			fmt.Println("Error: ", err)
			return err
		}
		if err = peer.Conn.WriteMessage(messageType, out); err != nil {
			fmt.Println(err)
			return err
		}
	} else {
		fmt.Println("Can not send offer to an unregistered peer")
		return err
	}
	return nil
}

func onLogin(data MessageInput, messageType int, conn *websocket.Conn) error {
	fmt.Println("User logged in")
	var err error
	var out []byte

	if _, isRegistered := USERS[data.Name]; isRegistered {
		out, err = json.Marshal(LoginResponse{Type: "login", Success: false})
	} else {
		USERS[data.Name] = User{Name: data.Name, Conn: conn}
		out, err = json.Marshal(LoginResponse{Type: "login", Success: true})
	}
	if err != nil {
		fmt.Println(err)
		return err
	}
	if err = conn.WriteMessage(messageType, out); err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

func connHandler(conn *websocket.Conn) {
	messageType, p, err := conn.ReadMessage()
	var message MessageInput

	if err != nil {
		fmt.Println(err)
		return
	}

	err = json.Unmarshal(p, &message)

	if err != nil {
		fmt.Println(err)
		return
	}
	messageInputType := message.Type

	switch messageInputType {
	case "login":
		onLogin(message, messageType, conn)
	case "offer":
		onOffer(message, messageType, conn)
	case "answer":
		fmt.Println("answer received")
	case "candidate":
		fmt.Println("candidate received")
	case "leave":
		fmt.Println("leave received")
	default:
		break
	}
}

//Catches HTTP Requests, upgrade them if needed and let connHandler managing the connection
func reqHandler(w http.ResponseWriter, r *http.Request) {
	//Upgrade a HTTP Request to get a pointer to a Conn
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	for {
		connHandler(conn)
	}
}

func main() {
	USERS = make(map[string]User)
	http.HandleFunc("/", reqHandler)
	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		fmt.Println("Error: " + err.Error())
	}
}
