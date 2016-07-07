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
	Offer     string `json:"offer,omitempty"`
	Answer    string `json:"answer,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// Define Offer sent back from the server
type OfferResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
}

//Ugrade policty from http request to websocket, to be defined
func checkOrigin(r *http.Request) bool {
	//For example: Check in a blacklist if the address is present
	//if blacklist_check(r.RemoteAddr) { return false }
	return true
}

func onLogin(data MessageInput, messageType int, conn *websocket.Conn) {
	fmt.Println("User logged in ")
	var err error
	var out []byte

	if _, isRegistered := USERS[data.Name]; isRegistered {
		out, err = json.Marshal(OfferResponse{Type: "login", Success: false})
	} else {
		USERS[data.Name] = User{Name: data.Name, Conn: conn}
		out, err = json.Marshal(OfferResponse{Type: "login", Success: true})
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	conn.WriteMessage(messageType, out)
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
		fmt.Println("offer received")
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
