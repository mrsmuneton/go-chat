package main

import (
	"log"
	"net/http"

	"github.com/elazarl/go-bindata-assetfs"
	"github.com/gorilla/websocket"

	"google.golang.org/appengine"

	mgo "gopkg.in/mgo.v2"
)

var clients = make(map[*websocket.Conn]bool) // connected clients
var broadcast = make(chan Message)           // broadcast channel
var c = connectToMongo()                     // establish connection to mongo

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
	appengine.Main() // Starts the server to receive requests

	// Use binary asset FileServer
	http.Handle("/",
		http.FileServer(
			&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo, Prefix: "public"}))

	// Configure websocket route
	http.HandleFunc("/ws", handleConnections)

	// Start listening for incoming chat messages
	go handleMessages()

	// Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :8000")
	err := http.ListenAndServe(":8000", nil)
	log.Println("after the server starts listening")
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

	// Populate the messages from the db
	go populateMessages()

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

func populateMessages() {
	var results []Message

	err := c.Find(nil).Sort("-timestamp").All(&results)
	if err != nil {
		log.Printf("error: %v", err)
	}

	for _, v := range results {
		log.Printf("v: %#+v\n", v)
		for client := range clients {
			err := client.WriteJSON(v)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
			}
		}
	}
}

func connectToMongo() *mgo.Collection {
	session, err := mgo.Dial("127.0.0.1")
	if err != nil {
		log.Printf("err: %#+v\n", err)
		panic(err)
	}
	session.SetMode(mgo.Monotonic, true)

	// this really needs to be fixed ray
	// defer session.Close()

	return session.DB("chat").C("convos")
}
