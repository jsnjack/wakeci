package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// Signing a person out closes the WebSockets they opened, so they stop
// receiving build updates; other people's stay open
func TestEndedSessionClosesWebSocket(t *testing.T) {
	oldSessions, oldConfig := GlobalSessionStorage, Config
	t.Cleanup(func() { GlobalSessionStorage, Config = oldSessions, oldConfig })
	newTestBuildEnv(t)
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]session)}
	GlobalSessionStorage.OnEnd = WSHub.CloseSessions

	router := chi.NewRouter()
	router.With(AuthMi).Get("/ws", HandleWS)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	dial := func(user string) *websocket.Conn {
		t.Helper()
		cookie := loginAs(t, user)
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
	// Sessions read the config's port; another test may have left none
	oldSessions, oldConfig := GlobalSessionStorage, Config
	t.Cleanup(func() { GlobalSessionStorage, Config = oldSessions, oldConfig })
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]session)}
	Config = &WakeConfig{}
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

// A client only gets messages about jobs it may read, and messages about the
// server
func TestHubSendsOnlyWhatTheClientMayRead(t *testing.T) {
	hub := newHub()
	go hub.run()
	// As ssoGroupsAccess makes them now, so the hub takes them as current
	version := ssoAccessVersion.Load()
	readDeploy := &Access{Grants: []Grant{{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}}}, version: version}
	runDeploy := &Access{Grants: []Grant{{Scopes: []string{ScopeRun}, Jobs: []string{"deploy"}}}, version: version}
	client := func(access *Access) *Client {
		c := &Client{hub: hub, send: make(chan []byte, 10), Logger: L, access: access,
			SubscribedTo: []string{"build:update:", "build:log:1", MsgTypeQueueStatus}}
		hub.register <- c
		return c
	}
	full, reader, runner, unknown := client(fullAccess), client(readDeploy), client(runDeploy), client(nil)
	for _, message := range []*MsgBroadcast{
		{Type: "build:update:1", Job: "deploy"},
		{Type: "build:update:2", Job: "nightly"},
		{Type: "build:log:1", Job: "deploy"},
		{Type: MsgTypeQueueStatus},
		{Type: "build:update:3"},
	} {
		hub.broadcast <- message
	}
	// Unregistering one more client makes sure the hub sent the above
	hub.unregister <- &Client{send: make(chan []byte)}

	received := func(c *Client) []string {
		var types []string
		for {
			select {
			case data := <-c.send:
				var message MsgBroadcast
				if err := json.Unmarshal(data, &message); err != nil {
					t.Fatalf("decode %q: %v", data, err)
				}
				types = append(types, message.Type)
			default:
				return types
			}
		}
	}
	tests := []struct {
		name   string
		client *Client
		want   []string
	}{
		{"admin password", full, []string{"build:update:1", "build:update:2", "build:log:1", MsgTypeQueueStatus, "build:update:3"}},
		{"read on deploy", reader, []string{"build:update:1", "build:log:1", MsgTypeQueueStatus}},
		{"run without read", runner, []string{MsgTypeQueueStatus}},
		{"no access", unknown, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := received(tt.client); !slices.Equal(got, tt.want) {
				t.Errorf("received %q, want %q", got, tt.want)
			}
		})
	}
	if strings.Contains(string(mustMarshal(t, &MsgBroadcast{Type: "build:update:1", Job: "deploy"})), "deploy") {
		t.Error("the job of a message is sent to the page")
	}
}

// After a group changed, the connections with limited access are closed, so
// the page opens them again with the new access
func TestHubClosesRestrictedClients(t *testing.T) {
	hub := newHub()
	go hub.run()
	client := func(access *Access) *Client {
		c := &Client{hub: hub, send: make(chan []byte, 1), Logger: L, access: access}
		hub.register <- c
		return c
	}
	full := client(fullAccess)
	everything := client(&Access{Grants: []Grant{{Scopes: []string{ScopeRead, ScopeRun, ScopeWrite}, AllJobs: true}}})
	hub.CloseRestricted()
	hub.unregister <- &Client{send: make(chan []byte)}

	closed := func(c *Client) bool {
		select {
		case _, ok := <-c.send:
			return !ok
		default:
			return false
		}
	}
	if closed(full) {
		t.Error("closed the admin password's connection")
	}
	// Even all permissions come from groups, which may have changed
	if !closed(everything) {
		t.Error("kept a connection whose access comes from groups")
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

// A connection whose access was computed before a group change, but that
// registers only after the hub closed the restricted ones, is closed too:
// otherwise it would keep the old access until it disconnects
func TestHubClosesClientsRegisteredAfterAnAccessChange(t *testing.T) {
	newTestBuildEnv(t)
	addTestGroup(t, "Ops", Grant{Scopes: []string{ScopeRead}, AllJobs: true})
	hub := newHub()
	go hub.run()
	oldChange := onSSOAccessChange
	onSSOAccessChange = hub.CloseRestricted
	t.Cleanup(func() { onSSOAccessChange = oldChange })

	before, err := ssoGroupsAccess([]string{"Ops"})
	if err != nil {
		t.Fatal(err)
	}
	// The group changes between AuthMi and the client's registration
	ssoAccessChanged()
	after, err := ssoGroupsAccess([]string{"Ops"})
	if err != nil {
		t.Fatal(err)
	}
	stale := &Client{hub: hub, send: make(chan []byte, 1), Logger: L, access: before}
	fresh := &Client{hub: hub, send: make(chan []byte, 1), Logger: L, access: after}
	full := &Client{hub: hub, send: make(chan []byte, 1), Logger: L, access: fullAccess}
	for _, c := range []*Client{stale, fresh, full} {
		hub.register <- c
	}
	hub.unregister <- &Client{send: make(chan []byte)}

	closed := func(c *Client) bool {
		select {
		case _, ok := <-c.send:
			return !ok
		default:
			return false
		}
	}
	if !closed(stale) {
		t.Error("kept a connection whose access predates the change")
	}
	if closed(fresh) || closed(full) {
		t.Error("closed a connection with current access")
	}
}
