package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// Signing a person out closes the WebSockets they opened, so they stop
// receiving build updates; other people's stay open
func TestEndedSessionClosesWebSocket(t *testing.T) {
	oldHub, oldSessions, oldConfig := WSHub, GlobalSessionStorage, Config
	t.Cleanup(func() { WSHub, GlobalSessionStorage, Config = oldHub, oldSessions, oldConfig })
	Config = &WakeConfig{}
	WSHub = newHub()
	go WSHub.run()
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]session)}
	GlobalSessionStorage.OnEnd = WSHub.CloseSessions

	router := chi.NewRouter()
	router.With(AuthMi).Get("/ws", HandleWS)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	dial := func(user string) *websocket.Conn {
		t.Helper()
		cookie, err := GlobalSessionStorage.New(httptest.NewRequest("POST", "/auth/login", nil), time.Hour, user)
		if err != nil {
			t.Fatalf("new session: %v", err)
		}
		header := http.Header{"Cookie": {cookie.Name + "=" + cookie.Value}}
		conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", header)
		if err != nil {
			t.Fatalf("dial as %s: %v", user, err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close handshake body: %v", err)
		}
		t.Cleanup(func() {
			if err := conn.Close(); err != nil {
				t.Logf("close ws: %v", err)
			}
		})
		return conn
	}
	alice, bob := dial("alice@example.com"), dial("bob@example.com")

	GlobalSessionStorage.DeleteUser("alice@example.com")

	if err := alice.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	_, _, err := alice.ReadMessage()
	var netErr net.Error
	if err == nil || (errors.As(err, &netErr) && netErr.Timeout()) {
		t.Errorf("alice's WebSocket after her sign-out: %v, want it closed", err)
	}
	if err := bob.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, _, err := bob.ReadMessage(); !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Errorf("bob's WebSocket: %v, want it still open", err)
	}
}

// A /ws request can pass AuthMi and only register after its session ended;
// the hub checks the session again when it registers the client
func TestHubRefusesClientOfEndedSession(t *testing.T) {
	oldSessions := GlobalSessionStorage
	t.Cleanup(func() { GlobalSessionStorage = oldSessions })
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]session)}
	hub := newHub()
	go hub.run()

	newSession := func(user string) string {
		cookie, err := GlobalSessionStorage.New(httptest.NewRequest("POST", "/auth/login", nil), time.Hour, user)
		if err != nil {
			t.Fatalf("new session: %v", err)
		}
		return cookie.Value
	}
	ended, valid := newSession("alice@example.com"), newSession("bob@example.com")
	GlobalSessionStorage.DeleteUser("alice@example.com")

	client := func(session string) *Client {
		c := &Client{hub: hub, send: make(chan []byte, 1), Logger: L, session: session}
		hub.register <- c
		return c
	}
	late, current, basic := client(ended), client(valid), client("")
	// Unregistering one more client makes sure the hub handled the three above
	hub.unregister <- &Client{send: make(chan []byte)}

	closed := func(c *Client) bool {
		select {
		case _, ok := <-c.send:
			return !ok
		default:
			return false
		}
	}
	if !closed(late) {
		t.Error("the client of an ended session was registered")
	}
	if closed(current) || closed(basic) {
		t.Errorf("valid clients closed: session %t, Basic auth %t", closed(current), closed(basic))
	}
}
