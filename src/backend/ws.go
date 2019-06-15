package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/julienschmidt/httprouter"
)

// ClientList is a list of connected clients
type ClientList struct {
	Clients []*Client
	sync.RWMutex
}

// Append creates new Client and appends it to the list
func (cl *ClientList) Append(ws *websocket.Conn) *Client {
	client := Client{
		WS:           ws,
		SubscribedTo: []MsgType{},
		Logger:       log.New(os.Stdout, "["+GenerateRandomString(5)+"] ", log.Lmicroseconds|log.Lshortfile),
	}
	cl.Lock()
	defer cl.Unlock()
	cl.Clients = append(cl.Clients, &client)
	client.Logger.Println("Client connected")
	return &client
}

// Remove removes a client from connected list
func (cl *ClientList) Remove(ws *websocket.Conn) {
	cl.Lock()
	defer cl.Unlock()
	for i, v := range cl.Clients {
		if v.WS == ws {
			v.Logger.Println("Client disconnected")
			cl.Clients[i] = nil
			cl.Clients = append(cl.Clients[:i], cl.Clients[i+1:]...)
			return
		}
	}
	Logger.Println("Unable to find a client")
}

// ConnectedClients ...
var ConnectedClients ClientList

// Client represents a websocket conection and subscriptions
type Client struct {
	WS           *websocket.Conn
	SubscribedTo []MsgType
	Logger       *log.Logger
}

// IsSubscribed checks if a client is subscribed for this type of messages
func (c *Client) IsSubscribed(tag MsgType) (bool, int) {
	for i, v := range c.SubscribedTo {
		if v == tag {
			return true, i
		}
	}
	return false, 0
}

// Subscribe subscribes a client to message
func (c *Client) Subscribe(mt MsgType) {
	ok, _ := c.IsSubscribed(mt)
	if !ok {
		c.SubscribedTo = append(c.SubscribedTo, mt)
		c.Logger.Printf("Has subscribed to %s\n", mt)
	}
}

// Unsubscribe ...
func (c *Client) Unsubscribe(mt MsgType) {
	ok, index := c.IsSubscribed(mt)
	if ok {
		c.SubscribedTo[index] = ""
		c.SubscribedTo = append(c.SubscribedTo[:index], c.SubscribedTo[index+1:]...)
		c.Logger.Printf("Has unsubscribed from %s\n", mt)
	}
}

// SendBuildInfo sends a message that contains information about the build to bootstrap
// the build page
func (c *Client) SendBuildInfo(id string) {

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
		c.Subscribe(MsgType(data.To))
		break
	case MsgTypeInUnsubscribe:
		var data InSubscribeData
		err := json.Unmarshal(msg.Data, &data)
		if err != nil {
			c.Logger.Println(err)
			return
		}
		c.Unsubscribe(MsgType(data.To))
		break
	default:
		c.Logger.Printf("Unhandled msg: %v\n", msg)
	}
}

// BroadcastChannel contains messages to send to all connected clients
var BroadcastChannel = make(chan *MsgBroadcast)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// BroadcastMessage broadcasts messages to all connected clients
func BroadcastMessage() {
	for {
		msg := <-BroadcastChannel
		msgB, err := json.Marshal(msg)
		if err != nil {
			Logger.Println(err)
		} else {
			for _, client := range ConnectedClients.Clients {
				ok, _ := client.IsSubscribed(msg.Type)
				if ok {
					err := client.WS.WriteMessage(websocket.TextMessage, msgB)
					if err != nil {
						Logger.Printf("error: %v\n", err)
					}
				}
			}
		}
	}
}

func handleWSConnection(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	// Upgrade initial GET request to a websocket
	Logger.Println("New ws connection...")
	ws, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		Logger.Fatal(err)
	}

	defer func() {
		ws.Close()
		ConnectedClients.Remove(ws)
	}()

	client := ConnectedClients.Append(ws)

	// Send information about all available jobs
	ws.WriteMessage(websocket.TextMessage, *GetAllJobsMessage())

	for {
		var msg MsgIncoming
		err := ws.ReadJSON(&msg)
		if err != nil {
			Logger.Println(err)
			ConnectedClients.Remove(ws)
			return
		}
		client.HandleIncomingMessage(&msg)
	}
}
