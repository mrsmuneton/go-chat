package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/websocket"
	mgo "gopkg.in/mgo.v2"
)

var clients = make(map[*websocket.Conn]bool) // connected clients
var broadcast = make(chan Message)           // broadcast channel

// Configure the upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Message object
type Message struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

func main() {
	// Use binary asset FileServer
	http.Handle("/",
		http.FileServer(
			&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo, Prefix: "public"}))

	// Configure websocket route
	http.HandleFunc("/ws", handleConnections)

	session, err := mgo.Dial("127.0.0.1")
	session.SetMode(mgo.Monotonic, true)
	c := session.DB("chat").C("convos")

	var results []Message

	c.Find(nil).Sort("-timestamp").All(&results)
	// whyno error handling Ray
	fmt.Println("Results All: ", results)
	for _, v := range results {
		log.Printf("v.Message: %#+v\n", v.Message)
	}

	// Start listening for incoming chat messages
	go handleMessages()

	// Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :8000")
	http.ListenAndServe(":8000", nil)
	// whyno error handling Ray
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error %v", err)
		return
	}
	// Make sure we close the connection when the function returns
	defer ws.Close()

	// Register our new client
	clients[ws] = true
	session, err := mgo.Dial("127.0.0.1")
	session.SetMode(mgo.Monotonic, true)
	c := session.DB("chat").C("convos")

	for {
		var msg Message
		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, ws)
			break
		}
		if err != nil {
			panic(err)
		}

		err = c.Insert(&Message{Email: msg.Email, Username: msg.Username, Message: msg.Message})

		// Send the newly received message to the broadcast channel
		broadcast <- msg
	}
	defer session.Close()
}

func handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		msg := <-broadcast
		// Send it out to every client that is currently connected
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}
