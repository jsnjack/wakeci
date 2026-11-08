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

	// Keys of sessions that ended, whose clients must be closed.
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
			// The session may have ended after AuthMi let the request in.
			// Ended sessions are deleted before the hub hears of them, so a
			// client registered after that is caught here.
			if client.session != "" && GlobalSessionStorage.Verify(client.session) != nil {
				client.Logger.Info("closing ws connection: its session ended")
				close(client.send)
				continue
			}
			client.Logger.Debug("ws connection registered")
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				client.Logger.Debug("ws connection unregistered")
				delete(h.clients, client)
				close(client.send)
			}
		case keys := <-h.endedSessions:
			ended := make(map[string]bool, len(keys))
			for _, key := range keys {
				ended[key] = true
			}
			for client := range h.clients {
				if client.session != "" && ended[sessionKey(client.session)] {
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

// CloseSessions closes the connections opened with the sessions, by
// sessionKey, e.g. when a person is signed out, so they stop receiving build
// updates
func (h *Hub) CloseSessions(keys []string) {
	h.endedSessions <- keys
}
