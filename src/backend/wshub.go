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

	// Sessions that ended, whose clients must be closed.
	endedSessions chan []string
}

func newHub() *Hub {
	L.Debug("starting wshub")
	return &Hub{
		broadcast:     make(chan *MsgBroadcast),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		endedSessions: make(chan []string),
		clients:       make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			client.Logger.Debug("ws connection registered")
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				client.Logger.Debug("ws connection unregistered")
				delete(h.clients, client)
				close(client.send)
			}
		case tokens := <-h.endedSessions:
			ended := make(map[string]bool, len(tokens))
			for _, token := range tokens {
				ended[token] = true
			}
			for client := range h.clients {
				if client.session != "" && ended[client.session] {
					client.Logger.Info("closing ws connection: its session ended")
					delete(h.clients, client)
					close(client.send)
				}
			}
		case message := <-h.broadcast:
			msgB, err := json.Marshal(message)
			if err != nil {
				L.Error("marshal broadcast message", "err", err)
			} else {
				for client := range h.clients {
					ok, _ := client.IsSubscribed(message.Type)
					if ok {
						select {
						case client.send <- msgB:
						default:
							client.Logger.Warn("send buffer full, dropping client")
							close(client.send)
							delete(h.clients, client)
						}
					}
				}
			}
		}
	}
}

// CloseSessions closes the connections opened with the sessions, e.g. when a
// person is signed out, so they stop receiving build updates
func (h *Hub) CloseSessions(tokens []string) {
	h.endedSessions <- tokens
}
