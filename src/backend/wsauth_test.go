package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	bolt "go.etcd.io/bbolt"
)

// dialWS opens /ws on server with the given credential headers
func dialWS(t *testing.T, server *httptest.Server, header http.Header) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial ws: %v (status %d)", err, status)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Logf("close ws: %v", err)
		}
	})
	return conn
}

// watchClose reports, once, when the server closes conn. gorilla/websocket
// connections are unusable after a read timeout, so a background reader is
// the only way to probe one without breaking it.
func watchClose(conn *websocket.Conn) <-chan error {
	closed := make(chan error, 1)
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				closed <- err
				return
			}
		}
	}()
	return closed
}

// TestWebSocketsCloseWhenCredentialsGo: a websocket keeps streaming build
// logs for as long as it's open, so it must close as soon as the credential
// it was opened with stops being good
func TestWebSocketsCloseWhenCredentialsGo(t *testing.T) {
	tests := []struct {
		name string
		// open returns the headers to dial with and the revocation to run
		open      func(t *testing.T, server *httptest.Server) (http.Header, func())
		wantClose bool
	}{
		{
			name: "nothing revoked",
			open: func(t *testing.T, server *httptest.Server) (http.Header, func()) {
				_, token := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())
				return http.Header{"Authorization": {"Bearer " + token}}, WSHub.Recheck
			},
		},
		{
			name: "api key revoked",
			open: func(t *testing.T, server *httptest.Server) (http.Header, func()) {
				key, token := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())
				session, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
				if err != nil {
					t.Fatalf("create session: %v", err)
				}
				return http.Header{"Authorization": {"Bearer " + token}}, func() {
					req, err := http.NewRequest(http.MethodDelete, server.URL+"/api/keys/"+key.ID, nil)
					if err != nil {
						t.Fatalf("build revoke request: %v", err)
					}
					req.AddCookie(session)
					resp, err := http.DefaultClient.Do(req)
					if err != nil || resp.StatusCode != http.StatusOK {
						t.Fatalf("revoke key: %v %v", resp, err)
					}
				}
			},
			wantClose: true,
		},
		{
			name: "logged out",
			open: func(t *testing.T, server *httptest.Server) (http.Header, func()) {
				session, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
				if err != nil {
					t.Fatalf("create session: %v", err)
				}
				return http.Header{"Cookie": {session.String()}}, func() {
					req, err := http.NewRequest(http.MethodPost, server.URL+"/auth/logout", nil)
					if err != nil {
						t.Fatalf("build logout request: %v", err)
					}
					req.AddCookie(session)
					resp, err := http.DefaultClient.Do(req)
					if err != nil || resp.StatusCode != http.StatusNoContent {
						t.Fatalf("log out: %v %v", resp, err)
					}
				}
			},
			wantClose: true,
		},
		{
			name: "google owner offboarded",
			open: func(t *testing.T, server *httptest.Server) (http.Header, func()) {
				Config.Google = GoogleConfig{ClientID: "client-id", ClientSecret: "client-secret", AllowedDomains: []string{"example.com"}}
				mustPutGoogleAccount(t, time.Now())
				_, token := mustCreateAPIKey(t, googleUser, []Scope{ScopeRead}, 0, time.Now())
				return http.Header{"Authorization": {"Bearer " + token}}, func() {
					account, err := getGoogleAccount(googleUser.ID)
					if err != nil || account == nil {
						t.Fatalf("getGoogleAccount: %v %v", account, err)
					}
					if err := offboardGoogleAccount(L, *account, "test"); err != nil {
						t.Fatalf("offboard: %v", err)
					}
				}
			},
			wantClose: true,
		},
		{
			name: "password session after password change",
			open: func(t *testing.T, server *httptest.Server) (http.Header, func()) {
				oldQueue := GlobalQueue
				GlobalQueue = &Queue{}
				t.Cleanup(func() { GlobalQueue = oldQueue })
				cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
				if err != nil {
					t.Fatalf("create session: %v", err)
				}
				return http.Header{"Cookie": {cookie.String()}}, func() {
					req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader("password=replacement&concurrentBuilds=1&buildHistorySize=10"))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					req.AddCookie(cookie)
					rec := httptest.NewRecorder()
					newRouter().ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("change password: %d %s", rec.Code, rec.Body.String())
					}
				}
			},
			wantClose: true,
		},
		{
			name: "shared password changed",
			open: func(t *testing.T, server *httptest.Server) (http.Header, func()) {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.SetBasicAuth("", "correct")
				return http.Header{"Authorization": {req.Header.Get("Authorization")}}, func() {
					err := DB.Update(func(tx *bolt.Tx) error {
						return tx.Bucket(GlobalBucket).Put([]byte("password"), []byte("new hash"))
					})
					if err != nil {
						t.Fatalf("change password: %v", err)
					}
					WSHub.Recheck()
				}
			},
			wantClose: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newAuthTestEnv(t)
			originalHub := WSHub
			WSHub = newHub()
			stop, stopped := make(chan struct{}), make(chan struct{})
			WSHub.stop = stop
			hub := WSHub
			go func() {
				defer close(stopped)
				hub.run()
			}()
			t.Cleanup(func() {
				close(stop)
				<-stopped
				WSHub = originalHub
			})
			handlerReturned := make(chan struct{})
			router := newRouter()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				router.ServeHTTP(w, r)
				if r.URL.Path == "/ws" {
					close(handlerReturned)
				}
			}))
			t.Cleanup(server.Close)

			header, revoke := tt.open(t, server)
			closed := watchClose(dialWS(t, server, header))
			<-handlerReturned
			select {
			case err := <-closed:
				t.Fatalf("websocket closed before anything was revoked: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			revoke()
			wait := 500 * time.Millisecond
			if tt.wantClose {
				wait = 2 * time.Second
			}
			select {
			case err := <-closed:
				if !tt.wantClose {
					t.Errorf("websocket closed although nothing was revoked: %v", err)
				}
			case <-time.After(wait):
				if tt.wantClose {
					t.Errorf("websocket still open %s after revocation", wait)
				}
			}
		})
	}
}

// TestBasicAuthWebSocketKeepsTheVerifiedHash: if the shared password changes
// between AuthMi checking it and the websocket being set up, the websocket
// must still close, rather than take the new password as the one it proved
func TestBasicAuthWebSocketKeepsTheVerifiedHash(t *testing.T) {
	newAuthTestEnv(t)
	var check func(time.Time) error
	var checkErr error
	handler := AuthMi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := DB.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(GlobalBucket).Put([]byte("password"), []byte("new hash"))
		})
		if err != nil {
			t.Errorf("change password: %v", err)
		}
		check, checkErr = credentialCheck(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.SetBasicAuth("", "correct")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if checkErr != nil {
		t.Fatalf("credentialCheck: %v", checkErr)
	}
	if err := check(time.Now()); !errors.Is(err, errCredentialRevoked) {
		t.Errorf("recheck after the password changed: %v, want errCredentialRevoked", err)
	}
}
