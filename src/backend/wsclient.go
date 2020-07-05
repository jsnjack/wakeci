package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
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
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	SubscribedTo []string
	Logger       *log.Logger

	mu deadlock.Mutex
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		var msg MsgIncoming
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Logger.Printf("error: %v", err)
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
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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
		if strings.HasPrefix(tag, v) {
			return true, i
		}
	}
	return false, 0
}

// Subscribe subscribes a client to message
func (c *Client) Subscribe(mt string) {
	ok, _ := c.IsSubscribed(mt)
	if !ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.SubscribedTo = append(c.SubscribedTo, mt)
		c.Logger.Printf("Has subscribed to %s\n", mt)
	}
}

// Unsubscribe ...
func (c *Client) Unsubscribe(mt string) {
	ok, index := c.IsSubscribed(mt)
	if ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.SubscribedTo[index] = ""
		c.SubscribedTo = append(c.SubscribedTo[:index], c.SubscribedTo[index+1:]...)
		c.Logger.Printf("Has unsubscribed from %s\n", mt)
	}
}

// HandleIncomingMessage ...
func (c *Client) HandleIncomingMessage(msg *MsgIncoming) {
	switch msg.Type {
	case MsgTypeInSubscribe:
		var data InSubscribeData
		err := json.Unmarshal(msg.Data, &data)
		if err != nil {
			c.Logger.Println(err)
			return
		}
		for _, item := range data.To {
			c.Subscribe(item)
		}
		break
	case MsgTypeInUnsubscribe:
		var data InSubscribeData
		err := json.Unmarshal(msg.Data, &data)
		if err != nil {
			c.Logger.Println(err)
			return
		}
		for _, item := range data.To {
			c.Unsubscribe(item)
		}
		break
	default:
		c.Logger.Printf("Unhandled msg: %v\n", msg)
	}
}

// HandleWS handles ws connection
func HandleWS(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Get IP address of a user
	addr := conn.RemoteAddr().String()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		Logger.Println(err)
		host = addr
	}

	logID := GenerateRandomString(5)

	client := &Client{
		hub:          WSHub,
		conn:         conn,
		send:         make(chan []byte, 256),
		SubscribedTo: []string{},
		Logger:       log.New(os.Stdout, "["+logID+" "+host+"] ", log.Lmicroseconds|log.Lshortfile),
	}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
