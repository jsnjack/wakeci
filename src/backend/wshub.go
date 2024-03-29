package main

import "encoding/json"

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan *MsgBroadcast

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

func newHub() *Hub {
	Logger.Println("Starting wshub...")
	return &Hub{
		broadcast:  make(chan *MsgBroadcast),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			client.Logger.Println("New ws connection registered")
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				client.Logger.Println("Connection unregistered")
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			msgB, err := json.Marshal(message)
			if err != nil {
				Logger.Println(err)
			} else {
				for client := range h.clients {
					ok, _ := client.IsSubscribed(message.Type)
					if ok {
						select {
						case client.send <- msgB:
						default:
							client.Logger.Println("Buffer is full")
							close(client.send)
							delete(h.clients, client)
						}
					}
				}
			}
		}
	}
}
