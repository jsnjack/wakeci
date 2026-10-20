package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sasha-s/go-deadlock"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkWebSocketOrigin,
}

func checkWebSocketOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}
	origin, err := url.Parse(origins[0])
	if err != nil || origin.Host == "" {
		return false
	}
	return strings.EqualFold(origin.Host, r.Host)
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	SubscribedTo []string
	Logger       *slog.Logger

	// revalidate re-checks the credential the connection opened with
	revalidate func(now time.Time) error
	// canSeeJob filters messages for keys limited to some jobs; nil for all
	canSeeJob func(name string) bool

	mu deadlock.Mutex
}

// resetReadDeadline extends the read deadline, used both on connect and on
// every pong. A failure here almost always means the connection is already
// gone, so it is only surfaced via --trace.
func (c *Client) resetReadDeadline() {
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		c.Logger.Log(context.Background(), LevelTrace, "set read deadline", "err", err)
	}
}

// resetWriteDeadline extends the write deadline before every write. A
// failure here almost always means the connection is already gone, so it is
// only surfaced via --trace.
func (c *Client) resetWriteDeadline() {
	if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		c.Logger.Log(context.Background(), LevelTrace, "set write deadline", "err", err)
	}
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		select {
		case c.hub.unregister <- c:
		case <-c.hub.stop:
		}
		if err := c.conn.Close(); err != nil {
			c.Logger.Log(context.Background(), LevelTrace, "close ws connection", "err", err)
		}
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.resetReadDeadline()
	c.conn.SetPongHandler(func(string) error { c.resetReadDeadline(); return nil })
	for {
		var msg MsgIncoming
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Logger.Error("read ws message", "err", err)
			}
			break
		}
		c.HandleIncomingMessage(&msg)
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		if err := c.conn.Close(); err != nil {
			c.Logger.Log(context.Background(), LevelTrace, "close ws connection", "err", err)
		}
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.resetWriteDeadline()
			if !ok {
				c.Logger.Debug("hub closed the channel")
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					c.Logger.Log(context.Background(), LevelTrace, "write ws close message", "err", err)
				}
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.Logger.Error("get ws writer", "err", err)
				return
			}
			if _, err := w.Write(message); err != nil {
				c.Logger.Error("write ws message", "err", err)
				return
			}

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				if _, err := w.Write(newline); err != nil {
					c.Logger.Error("write ws message", "err", err)
					return
				}
				if _, err := w.Write(<-c.send); err != nil {
					c.Logger.Error("write ws message", "err", err)
					return
				}
			}

			if err := w.Close(); err != nil {
				c.Logger.Error("close ws writer", "err", err)
				return
			}
		case <-ticker.C:
			c.resetWriteDeadline()
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.Logger.Error("write ping", "err", err)
				return
			}
		}
	}
}

// IsSubscribed checks if a client is subscribed for this type of messages
func (c *Client) IsSubscribed(tag string) (bool, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, v := range c.SubscribedTo {
		if tag == v || (strings.HasSuffix(v, ":") && strings.HasPrefix(tag, v)) {
			return true, i
		}
	}
	return false, 0
}

// Subscribe subscribes a client to message
func (c *Client) Subscribe(mt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, subscription := range c.SubscribedTo {
		if subscription == mt {
			return
		}
	}
	c.SubscribedTo = append(c.SubscribedTo, mt)
	c.Logger.Debug("subscribed", "to", mt)
}

// Unsubscribe ...
func (c *Client) Unsubscribe(mt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, subscription := range c.SubscribedTo {
		if subscription != mt {
			continue
		}
		c.SubscribedTo[index] = ""
		c.SubscribedTo = append(c.SubscribedTo[:index], c.SubscribedTo[index+1:]...)
		c.Logger.Debug("unsubscribed", "from", mt)
		return
	}
}

// HandleIncomingMessage ...
func (c *Client) HandleIncomingMessage(msg *MsgIncoming) {
	switch msg.Type {
	case MsgTypeInSubscribe:
		var data InSubscribeData
		err := json.Unmarshal(msg.Data, &data)
		if err != nil {
			c.Logger.Error("unmarshal subscribe message", "err", err)
			return
		}
		for _, item := range data.To {
			c.Subscribe(item)
		}
	case MsgTypeInUnsubscribe:
		var data InSubscribeData
		err := json.Unmarshal(msg.Data, &data)
		if err != nil {
			c.Logger.Error("unmarshal unsubscribe message", "err", err)
			return
		}
		for _, item := range data.To {
			c.Unsubscribe(item)
		}
	default:
		c.Logger.Warn("unhandled message", "type", msg.Type)
	}
}

// HandleWS handles ws connection
func HandleWS(w http.ResponseWriter, r *http.Request) {
	revalidate, err := credentialCheck(r)
	if err != nil {
		L.Error("ws credential check", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		L.Error("upgrade ws connection", "err", err)
		return
	}

	// Get IP address of a user
	addr := conn.RemoteAddr().String()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		L.Error("split remote addr", "addr", addr, "err", err)
		host = addr
	}

	logID := GenerateRandomString(5)

	client := &Client{
		hub:          WSHub,
		conn:         conn,
		send:         make(chan []byte, 1024),
		SubscribedTo: []string{},
		Logger:       L.With("logID", logID, "host", host),
		revalidate:   revalidate,
		canSeeJob:    jobFilter(r),
	}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
