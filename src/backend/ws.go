package main

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
)

// List of connected clients
var clients []*websocket.Conn

// BroadcastChannel contains messages to send to all connected clients
var BroadcastChannel = make(chan []byte)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// BroadcastMessages broadcasts messages to all connected clients
func BroadcastMessages() {
	for {
		msg := <-BroadcastChannel
		for _, client := range clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				Logger.Printf("error: %v\n", err)
			}
		}
	}
}

func handleWSConnection(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	// Upgrade initial GET request to a websocket
	Logger.Println("New ws connection")
	ws, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		Logger.Fatal(err)
	}
	// Register our new client
	clients = append(clients, ws)

	defer func() {
		ws.Close()
		removeConnection(ws)
	}()

	// Send information about all available jobs
	ws.WriteMessage(websocket.TextMessage, *GetAllJobsMessage())

	for {
		var msg interface{}
		err := ws.ReadJSON(&msg)
		if err != nil {
			Logger.Println(err)
			removeConnection(ws)
			return
		}
		Logger.Println("Unhandled message", msg)
	}
}

func removeConnection(conn *websocket.Conn) {
	var updatedClients []*websocket.Conn
	for _, item := range clients {
		if item != conn {
			updatedClients = append(updatedClients, item)
		}
	}
	clients = updatedClients
}
