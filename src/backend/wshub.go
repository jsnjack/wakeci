package main

import (
	"encoding/json"
	"time"
)

// wsRevalidateInterval bounds how long a websocket outlives its credential:
// sessions expire, keys expire and Google accounts go stale on their own
const wsRevalidateInterval = 30 * time.Second

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// stop ends the loop and closes active clients; nil keeps it running.
	stop <-chan struct{}

	// Registered clients.
	clients map[*Client]bool

	// recheck asks for every client's credential to be re-checked now
	recheck chan struct{}

	// Inbound messages from the clients.
	broadcast chan *MsgBroadcast

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

func newHub() *Hub {
	L.Debug("starting wshub")
	return &Hub{
		broadcast:  make(chan *MsgBroadcast),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		recheck:    make(chan struct{}, 1),
	}
}

// Recheck closes, soon, every websocket whose credential stopped being good.
// Call it whenever one may have been revoked: logout, key revocation, Google
// offboarding, a password change.
func (h *Hub) Recheck() {
	if h == nil {
		return
	}
	select {
	case h.recheck <- struct{}{}:
	default:
		// A recheck is already pending and will see this change too
	}
}

// revalidate drops every client whose credential no longer checks out
func (h *Hub) revalidate(now time.Time) {
	for client := range h.clients {
		if client.revalidate == nil {
			continue
		}
		err := client.revalidate(now)
		if err == nil {
			continue
		}
		if !isRevoked(err) {
			// Can't tell; the next recheck will try again
			client.Logger.Warn("recheck ws credential", "err", err)
			continue
		}
		client.Logger.Warn("closing ws connection: credential revoked", "err", err)
		delete(h.clients, client)
		close(client.send)
	}
}

func (h *Hub) run() {
	ticker := time.NewTicker(wsRevalidateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.stop:
			for client := range h.clients {
				close(client.send)
			}
			return
		case <-ticker.C:
			h.revalidate(time.Now())
		case <-h.recheck:
			h.revalidate(time.Now())
		case client := <-h.register:
			client.Logger.Debug("ws connection registered")
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				client.Logger.Debug("ws connection unregistered")
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			msgB, err := json.Marshal(message)
			if err != nil {
				L.Error("marshal broadcast message", "err", err)
			} else {
				for client := range h.clients {
					if client.canSeeJob != nil && !client.canSeeJob(message.job) {
						continue
					}
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
