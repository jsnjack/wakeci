package main

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

const (
	testIDPEntityID   = "https://accounts.google.com/o/saml2?idpid=C0test"
	testIDPSSOURL     = "https://accounts.google.com/o/saml2/idp?idpid=C0test"
	testSSOEntityID   = "https://wakeci.example.com"
	testSSOACSURL     = "https://wakeci.example.com/auth/sso/acs"
	testSSOEmail      = "alice@example.com"
	testLoginRedirect = "/build/7"

	samlAssertionNS = "urn:oasis:names:tc:SAML:2.0:assertion"
	samlProtocolNS  = "urn:oasis:names:tc:SAML:2.0:protocol"
	samlSuccess     = "urn:oasis:names:tc:SAML:2.0:status:Success"
)

// testSSOGroups are the groups allowed to log in, unless a test says otherwise
var testSSOGroups = []string{"Developers", "Ops"}

// Login page URLs the ACS endpoint redirects to on failure
var (
	wantSSOError       = "/login?" + url.Values{"error": {loginErrorSSO}, "redirect": {testLoginRedirect}}.Encode()
	wantSSONotAllowed  = "/login?" + url.Values{"error": {loginErrorSSONotAllowed}, "redirect": {testLoginRedirect}}.Encode()
	wantSSOErrorNoPage = "/login?error=" + loginErrorSSO
)

// testIDPKey signs the test identity provider's responses; otherIDPKey
// stands in for anyone else
var (
	testIDPKey  = dsig.RandomKeyStoreForTest()
	otherIDPKey = dsig.RandomKeyStoreForTest()
)

// testMetadata builds identity provider metadata shaped like Google's
type testMetadata struct {
	entityID string
	keyUse   string
	cert     string // base64, may be wrapped over several lines
	binding  string
}

func newTestMetadata(t *testing.T) testMetadata {
	t.Helper()
	_, cert, err := testIDPKey.GetKeyPair()
	if err != nil {
		t.Fatalf("get test key pair: %v", err)
	}
	// Wrap and indent like metadata files do
	encoded := base64.StdEncoding.EncodeToString(cert)
	var wrapped strings.Builder
	for len(encoded) > 64 {
		wrapped.WriteString(encoded[:64] + "\n\t\t\t")
		encoded = encoded[64:]
	}
	wrapped.WriteString(encoded)
	return testMetadata{
		entityID: testIDPEntityID,
		keyUse:   "signing",
		cert:     wrapped.String(),
		binding:  "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect",
	}
}

func (m testMetadata) xml() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s">
  <md:IDPSSODescriptor WantAuthnRequestsSigned="false" protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <md:KeyDescriptor use="%s">
      <ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#">
        <ds:X509Data>
          <ds:X509Certificate>%s</ds:X509Certificate>
        </ds:X509Data>
      </ds:KeyInfo>
    </md:KeyDescriptor>
    <md:NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified</md:NameIDFormat>
    <md:SingleSignOnService Binding="%s" Location="%s"/>
  </md:IDPSSODescriptor>
</md:EntityDescriptor>
`, m.entityID, m.keyUse, m.cert, m.binding, testIDPSSOURL)
}

func newSSOTestEnv(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "idp.xml")
	if err := os.WriteFile(path, []byte(newTestMetadata(t).xml()), 0600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	auth, err := NewSSOAuth(SSOConfig{
		IDPMetadata:   path,
		ACSURL:        testSSOACSURL,
		EntityID:      testSSOEntityID,
		allowedGroups: testSSOGroups,
	})
	if err != nil {
		t.Fatalf("NewSSOAuth: %v", err)
	}

	originalConfig := Config
	originalSessions := GlobalSessionStorage
	originalSSO := GlobalSSOAuth
	Config = &WakeConfig{Port: "8081"}
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]time.Time)}
	GlobalSSOAuth = auth
	t.Cleanup(func() {
		Config = originalConfig
		GlobalSessionStorage = originalSessions
		GlobalSSOAuth = originalSSO
	})
}

// testSAMLResponse builds a SAML response like the identity provider posts
type testSAMLResponse struct {
	issuer       string
	inResponseTo string
	recipient    string
	audience     string // empty leaves out the audience restriction
	notBefore    time.Time
	notOnOrAfter time.Time
	nameID       string
	groups       []string // nil leaves out the attribute statement
	status       string
	signer       dsig.X509KeyStore // nil leaves the response unsigned
	signResponse bool              // sign the response instead of the assertion
	assertions   int
	tamper       func(t *testing.T, response *etree.Element) // runs after signing
}

func newTestSAMLResponse(requestID string) testSAMLResponse {
	now := time.Now()
	return testSAMLResponse{
		issuer:       testIDPEntityID,
		inResponseTo: requestID,
		recipient:    testSSOACSURL,
		audience:     testSSOEntityID,
		notBefore:    now.Add(-time.Minute),
		notOnOrAfter: now.Add(5 * time.Minute),
		nameID:       testSSOEmail,
		groups:       []string{"Sales", "Developers"},
		status:       samlSuccess,
		signer:       testIDPKey,
		assertions:   1,
	}
}

func samlTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func (tr testSAMLResponse) assertion() *etree.Element {
	a := etree.NewElement("saml:Assertion")
	a.CreateAttr("xmlns:saml", samlAssertionNS)
	a.CreateAttr("ID", "_"+rand.Text())
	a.CreateAttr("Version", "2.0")
	a.CreateAttr("IssueInstant", samlTime(time.Now()))
	a.CreateElement("saml:Issuer").SetText(tr.issuer)

	subject := a.CreateElement("saml:Subject")
	nameID := subject.CreateElement("saml:NameID")
	nameID.CreateAttr("Format", "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress")
	nameID.SetText(tr.nameID)
	confirmation := subject.CreateElement("saml:SubjectConfirmation")
	confirmation.CreateAttr("Method", "urn:oasis:names:tc:SAML:2.0:cm:bearer")
	data := confirmation.CreateElement("saml:SubjectConfirmationData")
	if tr.inResponseTo != "" {
		data.CreateAttr("InResponseTo", tr.inResponseTo)
	}
	data.CreateAttr("NotOnOrAfter", samlTime(tr.notOnOrAfter))
	data.CreateAttr("Recipient", tr.recipient)

	conditions := a.CreateElement("saml:Conditions")
	conditions.CreateAttr("NotBefore", samlTime(tr.notBefore))
	conditions.CreateAttr("NotOnOrAfter", samlTime(tr.notOnOrAfter))
	if tr.audience != "" {
		conditions.CreateElement("saml:AudienceRestriction").CreateElement("saml:Audience").SetText(tr.audience)
	}

	if tr.groups != nil {
		attribute := a.CreateElement("saml:AttributeStatement").CreateElement("saml:Attribute")
		attribute.CreateAttr("Name", ssoGroupsAttribute)
		for _, group := range tr.groups {
			attribute.CreateElement("saml:AttributeValue").SetText(group)
		}
	}
	authn := a.CreateElement("saml:AuthnStatement")
	authn.CreateAttr("AuthnInstant", samlTime(time.Now()))
	authn.CreateElement("saml:AuthnContext").CreateElement("saml:AuthnContextClassRef").
		SetText("urn:oasis:names:tc:SAML:2.0:ac:classes:unspecified")
	return a
}

// signSAML signs el the way identity providers do: enveloped, exclusive
// canonicalization, the signature right after the Issuer
func signSAML(t *testing.T, key dsig.X509KeyStore, el *etree.Element) *etree.Element {
	t.Helper()
	ctx := dsig.NewDefaultSigningContext(key)
	ctx.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
	sig, err := ctx.ConstructSignature(el, true)
	if err != nil {
		t.Fatalf("sign %s: %v", el.Tag, err)
	}
	signed := el.Copy()
	signed.InsertChildAt(1, sig)
	return signed
}

func (tr testSAMLResponse) encode(t *testing.T) string {
	t.Helper()
	response := etree.NewElement("samlp:Response")
	response.CreateAttr("xmlns:samlp", samlProtocolNS)
	response.CreateAttr("xmlns:saml", samlAssertionNS)
	response.CreateAttr("ID", "_"+rand.Text())
	response.CreateAttr("Version", "2.0")
	response.CreateAttr("IssueInstant", samlTime(time.Now()))
	response.CreateAttr("Destination", tr.recipient)
	if tr.inResponseTo != "" {
		response.CreateAttr("InResponseTo", tr.inResponseTo)
	}
	response.CreateElement("saml:Issuer").SetText(tr.issuer)
	response.CreateElement("samlp:Status").CreateElement("samlp:StatusCode").CreateAttr("Value", tr.status)
	for range tr.assertions {
		assertion := tr.assertion()
		if tr.signer != nil && !tr.signResponse {
			assertion = signSAML(t, tr.signer, assertion)
		}
		response.AddChild(assertion)
	}
	if tr.signer != nil && tr.signResponse {
		response = signSAML(t, tr.signer, response)
	}
	if tr.tamper != nil {
		tr.tamper(t, response)
	}
	doc := etree.NewDocument()
	doc.SetRoot(response)
	xml, err := doc.WriteToString()
	if err != nil {
		t.Fatalf("write response: %v", err)
	}
	return base64.StdEncoding.EncodeToString([]byte(xml))
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func startSSOLogin(t *testing.T, redirect string) (*url.URL, *http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, ssoLoginPath+"?redirect="+url.QueryEscape(redirect), nil)
	rec := httptest.NewRecorder()
	HandleSSOLogIn(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("login status = %d, want %d", rec.Code, http.StatusFound)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	cookie := findCookie(rec.Result().Cookies(), ssoStateCookieName)
	if cookie == nil {
		t.Fatal("no login state cookie")
	}
	return location, cookie
}

func openTestLoginState(t *testing.T, c *http.Cookie) *ssoLoginState {
	t.Helper()
	login, err := GlobalSSOAuth.openLoginState(c.Value, time.Now())
	if err != nil {
		t.Fatalf("open login state: %v", err)
	}
	return login
}

// decodeAuthnRequest reads the AuthnRequest from an HTTP-Redirect binding URL
func decodeAuthnRequest(t *testing.T, location *url.URL) *etree.Element {
	t.Helper()
	deflated, err := base64.StdEncoding.DecodeString(location.Query().Get("SAMLRequest"))
	if err != nil {
		t.Fatalf("decode SAMLRequest: %v", err)
	}
	raw, err := io.ReadAll(flate.NewReader(bytes.NewReader(deflated)))
	if err != nil {
		t.Fatalf("inflate SAMLRequest: %v", err)
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(raw); err != nil {
		t.Fatalf("parse SAMLRequest: %v", err)
	}
	return doc.Root()
}

func TestSSOLogInRedirect(t *testing.T) {
	newSSOTestEnv(t)
	location, cookie := startSSOLogin(t, testLoginRedirect)
	login := openTestLoginState(t, cookie)
	request := decodeAuthnRequest(t, location)
	location.RawQuery = url.Values{"idpid": location.Query()["idpid"]}.Encode()

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"identity provider URL", location.String(), testIDPSSOURL},
		{"request", request.Tag, "AuthnRequest"},
		{"request ID in cookie", request.SelectAttrValue("ID", ""), login.RequestID},
		{"ACS URL", request.SelectAttrValue("AssertionConsumerServiceURL", ""), testSSOACSURL},
		{"binding", request.SelectAttrValue("ProtocolBinding", ""), "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"},
		{"destination", request.SelectAttrValue("Destination", ""), testIDPSSOURL},
		{"issuer", request.FindElement("Issuer").Text(), testSSOEntityID},
		{"redirect", login.Redirect, testLoginRedirect},
		{"cookie path", cookie.Path, "/auth/sso/"},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("got %q, want %q", c.got, c.want)
			}
		})
	}
	t.Run("cookie attributes", func(t *testing.T) {
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteNoneMode || cookie.MaxAge <= 0 {
			t.Errorf("cookie HttpOnly=%t Secure=%t SameSite=%v MaxAge=%d, want HttpOnly, Secure, None, positive MaxAge",
				cookie.HttpOnly, cookie.Secure, cookie.SameSite, cookie.MaxAge)
		}
	})
}

func TestSSOLoginStateRedirect(t *testing.T) {
	tests := []struct {
		name     string
		redirect string
		want     string
	}{
		{"page on this server", testLoginRedirect, testLoginRedirect},
		{"other site", "//evil.example.com", "/"},
		{"too long", "/jobs?q=" + strings.Repeat("a", 3000), "/"},
		{"longest allowed", "/" + strings.Repeat("a", maxRedirectPathLength-1), "/" + strings.Repeat("a", maxRedirectPathLength-1)},
		// Allowed, but JSON writes each "&" as \u0026, six times its length
		{"too long once escaped", "/?" + strings.Repeat("&", maxRedirectPathLength-2), "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newSSOTestEnv(t)
			_, cookie := startSSOLogin(t, tt.redirect)
			if got := openTestLoginState(t, cookie).Redirect; got != tt.want {
				t.Errorf("stored redirect = %q, want %q", got, tt.want)
			}
			if len(cookie.Value) > maxLoginStateCookieLength {
				t.Errorf("cookie value is %d bytes, want at most %d", len(cookie.Value), maxLoginStateCookieLength)
			}
		})
	}
}

func TestOpenLoginState(t *testing.T) {
	newSSOTestEnv(t)
	now := time.Now()
	seal := func(t *testing.T, auth *SSOAuth, login ssoLoginState) string {
		t.Helper()
		value, err := auth.sealLoginState(login)
		if err != nil {
			t.Fatalf("seal: %v", err)
		}
		return value
	}
	valid := ssoLoginState{RequestID: "_request", Redirect: testLoginRedirect, Expires: now.Add(time.Minute).Unix()}
	enc := base64.RawURLEncoding
	withMAC := func(payload []byte) string {
		return enc.EncodeToString(payload) + "." + enc.EncodeToString(GlobalSSOAuth.loginStateMAC(payload))
	}
	otherAuth := &SSOAuth{stateKey: []byte("key-from-before-a-restart")}

	tests := []struct {
		name         string
		value        string
		wantRedirect string // empty means the state must be rejected
	}{
		{name: "valid", value: seal(t, GlobalSSOAuth, valid), wantRedirect: testLoginRedirect},
		{name: "not sealed", value: "!!!"},
		{name: "payload not base64", value: "!!!." + strings.Split(seal(t, GlobalSSOAuth, valid), ".")[1]},
		{name: "signature not base64", value: strings.Split(seal(t, GlobalSSOAuth, valid), ".")[0] + ".!!!"},
		{name: "payload changed", value: func() string {
			sealed := seal(t, GlobalSSOAuth, valid)
			forged, err := json.Marshal(ssoLoginState{RequestID: "_stolen", Expires: valid.Expires})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			return enc.EncodeToString(forged) + "." + strings.Split(sealed, ".")[1]
		}()},
		{name: "signed with another key", value: seal(t, otherAuth, valid)},
		{name: "expired", value: seal(t, GlobalSSOAuth, ssoLoginState{RequestID: "_request", Expires: now.Unix()})},
		{name: "no request ID", value: seal(t, GlobalSSOAuth, ssoLoginState{Expires: valid.Expires})},
		{name: "signed but not JSON", value: withMAC([]byte("not json"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			login, err := GlobalSSOAuth.openLoginState(tt.value, now)
			if tt.wantRedirect == "" {
				if err == nil {
					t.Errorf("accepted login state %+v", login)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if login.Redirect != tt.wantRedirect {
				t.Errorf("redirect = %q, want %q", login.Redirect, tt.wantRedirect)
			}
		})
	}
}

func TestSSOACS(t *testing.T) {
	tests := []struct {
		name         string
		response     func(r *testSAMLResponse)
		request      func(t *testing.T, form url.Values, c *http.Cookie) (url.Values, *http.Cookie)
		wantLocation string
		wantSession  bool
	}{
		{name: "member of an allowed group", wantLocation: testLoginRedirect, wantSession: true},
		{
			name:         "member of the second allowed group",
			response:     func(r *testSAMLResponse) { r.groups = []string{"Ops"} },
			wantLocation: testLoginRedirect,
			wantSession:  true,
		},
		{
			name:         "group in other case",
			response:     func(r *testSAMLResponse) { r.groups = []string{" developers "} },
			wantLocation: testLoginRedirect,
			wantSession:  true,
		},
		{
			name:         "whole response signed",
			response:     func(r *testSAMLResponse) { r.signResponse = true },
			wantLocation: testLoginRedirect,
			wantSession:  true,
		},
		{
			name:         "in no allowed group",
			response:     func(r *testSAMLResponse) { r.groups = []string{"Sales"} },
			wantLocation: wantSSONotAllowed,
		},
		{
			name:         "no groups attribute",
			response:     func(r *testSAMLResponse) { r.groups = nil },
			wantLocation: wantSSONotAllowed,
		},
		{
			name: "group added after signing",
			response: func(r *testSAMLResponse) {
				r.groups = []string{"Sales"}
				r.tamper = func(t *testing.T, response *etree.Element) {
					attribute := response.FindElement("//saml:Attribute")
					if attribute == nil {
						t.Fatal("no groups attribute to tamper with")
					}
					attribute.CreateElement("saml:AttributeValue").SetText("Developers")
				}
			},
			wantLocation: wantSSOError,
		},
		{
			name: "forged assertion next to the signed one",
			response: func(r *testSAMLResponse) {
				r.groups = []string{"Sales"}
				r.tamper = func(t *testing.T, response *etree.Element) {
					forged := response.FindElement("saml:Assertion").Copy()
					forged.RemoveChild(forged.FindElement("ds:Signature"))
					forged.FindElement("//saml:AttributeValue").SetText("Developers")
					response.InsertChildAt(2, forged)
				}
			},
			wantLocation: wantSSOError,
		},
		{
			name:         "signed by another key",
			response:     func(r *testSAMLResponse) { r.signer = otherIDPKey },
			wantLocation: wantSSOError,
		},
		{
			name:         "unsigned",
			response:     func(r *testSAMLResponse) { r.signer = nil },
			wantLocation: wantSSOError,
		},
		{
			name:         "two assertions",
			response:     func(r *testSAMLResponse) { r.assertions = 2 },
			wantLocation: wantSSOError,
		},
		{
			name:         "for another service provider",
			response:     func(r *testSAMLResponse) { r.audience = "https://other.example.com" },
			wantLocation: wantSSOError,
		},
		{
			name:         "no audience restriction",
			response:     func(r *testSAMLResponse) { r.audience = "" },
			wantLocation: wantSSOError,
		},
		{
			name:         "for another ACS URL",
			response:     func(r *testSAMLResponse) { r.recipient = "https://other.example.com/auth/sso/acs" },
			wantLocation: wantSSOError,
		},
		{
			name:         "from another identity provider",
			response:     func(r *testSAMLResponse) { r.issuer = "https://idp.example.com" },
			wantLocation: wantSSOError,
		},
		{
			name:         "expired",
			response:     func(r *testSAMLResponse) { r.notOnOrAfter = time.Now().Add(-time.Minute) },
			wantLocation: wantSSOError,
		},
		{
			name:         "not valid yet",
			response:     func(r *testSAMLResponse) { r.notBefore = time.Now().Add(5 * time.Minute) },
			wantLocation: wantSSOError,
		},
		{
			name:         "answers another login request",
			response:     func(r *testSAMLResponse) { r.inResponseTo = "_another-request" },
			wantLocation: wantSSOError,
		},
		{
			name:         "failed at the identity provider",
			response:     func(r *testSAMLResponse) { r.status = "urn:oasis:names:tc:SAML:2.0:status:Requester" },
			wantLocation: wantSSOError,
		},
		{
			name:         "no NameID",
			response:     func(r *testSAMLResponse) { r.nameID = "" },
			wantLocation: wantSSOError,
		},
		{
			name:         "unsolicited",
			response:     func(r *testSAMLResponse) { r.inResponseTo = "" },
			wantLocation: ssoLoginPath,
		},
		{
			name:     "unsolicited, without login state",
			response: func(r *testSAMLResponse) { r.inResponseTo = "" },
			request: func(_ *testing.T, form url.Values, _ *http.Cookie) (url.Values, *http.Cookie) {
				return form, nil
			},
			wantLocation: ssoLoginPath,
		},
		{
			// Only the assertion is signed, so InResponseTo on the Response
			// element around it can be set by anyone
			name: "InResponseTo only on the unsigned response",
			response: func(r *testSAMLResponse) {
				requestID := r.inResponseTo
				r.inResponseTo = ""
				r.tamper = func(_ *testing.T, response *etree.Element) {
					response.CreateAttr("InResponseTo", requestID)
				}
			},
			wantLocation: ssoLoginPath,
		},
		{
			name:         "unsolicited and signed by another key",
			response:     func(r *testSAMLResponse) { r.inResponseTo = ""; r.signer = otherIDPKey },
			wantLocation: wantSSOError,
		},
		{
			name: "no login state cookie",
			request: func(_ *testing.T, form url.Values, _ *http.Cookie) (url.Values, *http.Cookie) {
				return form, nil
			},
			wantLocation: wantSSOErrorNoPage,
		},
		{
			name: "login state cookie not sealed",
			request: func(_ *testing.T, form url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				broken := *c
				broken.Value = "!!!"
				return form, &broken
			},
			wantLocation: wantSSOErrorNoPage,
		},
		{
			name: "response not base64",
			request: func(_ *testing.T, form url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				form.Set("SAMLResponse", "!!!")
				return form, c
			},
			wantLocation: wantSSOError,
		},
		{
			name: "response too large",
			request: func(_ *testing.T, form url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				form.Set("SAMLResponse", strings.Repeat("a", maxSAMLResponseSize))
				return form, c
			},
			wantLocation: wantSSOErrorNoPage,
		},
		{
			name: "no response",
			request: func(_ *testing.T, form url.Values, c *http.Cookie) (url.Values, *http.Cookie) {
				form.Del("SAMLResponse")
				return form, c
			},
			wantLocation: wantSSOError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newSSOTestEnv(t)
			_, stateCookie := startSSOLogin(t, testLoginRedirect)

			response := newTestSAMLResponse(openTestLoginState(t, stateCookie).RequestID)
			if tt.response != nil {
				tt.response(&response)
			}
			form := url.Values{"SAMLResponse": {response.encode(t)}}
			cookie := stateCookie
			if tt.request != nil {
				form, cookie = tt.request(t, form, cookie)
			}
			req := httptest.NewRequest(http.MethodPost, SSOACSPath, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if cookie != nil {
				req.AddCookie(cookie)
			}
			rec := httptest.NewRecorder()
			HandleSSOACS(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
			}
			if got := rec.Header().Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}
			cleared := findCookie(rec.Result().Cookies(), ssoStateCookieName)
			if cleared == nil || cleared.MaxAge >= 0 {
				t.Errorf("login state cookie not cleared: %+v", cleared)
			}

			session := findCookie(rec.Result().Cookies(), "session")
			if !tt.wantSession {
				if session != nil {
					t.Errorf("unexpected session cookie")
				}
				return
			}
			if session == nil {
				t.Fatal("no session cookie")
			}
			if err := GlobalSessionStorage.Verify(session.Value); err != nil {
				t.Errorf("session not stored: %v", err)
			}
			if ttl := time.Until(session.Expires); ttl > SSOSessionTTL || ttl < SSOSessionTTL-time.Minute {
				t.Errorf("session expires in %v, want about %v", ttl, SSOSessionTTL)
			}
		})
	}
}

func TestSSOACSReplay(t *testing.T) {
	newSSOTestEnv(t)
	_, cookie := startSSOLogin(t, testLoginRedirect)
	form := url.Values{"SAMLResponse": {newTestSAMLResponse(openTestLoginState(t, cookie).RequestID).encode(t)}}

	// The same response and cookie, as captured in a HAR file or proxy log
	for i, want := range []string{testLoginRedirect, wantSSOError} {
		req := httptest.NewRequest(http.MethodPost, SSOACSPath, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		HandleSSOACS(rec, req)
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("post %d: Location = %q, want %q", i+1, got, want)
		}
	}
}

func TestUseRequestForgetsExpired(t *testing.T) {
	s := &SSOAuth{usedRequests: make(map[string]time.Time)}
	now := time.Now()
	if !s.useRequest("_a", now) || s.useRequest("_a", now) {
		t.Fatal("want first use accepted and second refused")
	}
	if !s.useRequest("_b", now.Add(ssoStateTTL)) {
		t.Fatal("new request refused")
	}
	if _, kept := s.usedRequests["_a"]; kept || len(s.usedRequests) != 1 {
		t.Errorf("used requests = %v, want only _b", s.usedRequests)
	}
}

func TestParseIDPMetadata(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(m *testMetadata)
		raw     string // replaces the metadata built from edit
		wantErr bool
	}{
		{name: "google-like metadata"},
		{name: "key without use", edit: func(m *testMetadata) { m.keyUse = "" }},
		{name: "not XML", raw: "not xml", wantErr: true},
		{name: "no entity ID", edit: func(m *testMetadata) { m.entityID = "" }, wantErr: true},
		{
			name:    "service provider metadata",
			raw:     `<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="x"/>`,
			wantErr: true,
		},
		{
			name:    "POST binding only",
			edit:    func(m *testMetadata) { m.binding = "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" },
			wantErr: true,
		},
		{name: "encryption key only", edit: func(m *testMetadata) { m.keyUse = "encryption" }, wantErr: true},
		{name: "certificate not base64", edit: func(m *testMetadata) { m.cert = "!!!" }, wantErr: true},
		{
			name:    "certificate not a certificate",
			edit:    func(m *testMetadata) { m.cert = base64.StdEncoding.EncodeToString([]byte("not a cert")) },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestMetadata(t)
			if tt.edit != nil {
				tt.edit(&m)
			}
			data := m.xml()
			if tt.raw != "" {
				data = tt.raw
			}
			idp, err := parseIDPMetadata([]byte(data))
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected an error, got %+v", idp)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_, cert, err := testIDPKey.GetKeyPair()
			if err != nil {
				t.Fatalf("get key pair: %v", err)
			}
			if idp.entityID != testIDPEntityID || idp.ssoURL != testIDPSSOURL ||
				len(idp.certs) != 1 || !bytes.Equal(idp.certs[0].Raw, cert) {
				t.Errorf("got entityID %q, ssoURL %q, %d certs", idp.entityID, idp.ssoURL, len(idp.certs))
			}
		})
	}
}

func TestNewSSOAuthMissingMetadata(t *testing.T) {
	_, err := NewSSOAuth(SSOConfig{
		IDPMetadata:   filepath.Join(t.TempDir(), "missing.xml"),
		ACSURL:        testSSOACSURL,
		EntityID:      testSSOEntityID,
		allowedGroups: testSSOGroups,
	})
	if err == nil {
		t.Error("expected an error for a missing metadata file")
	}
}

func TestSSOHandlersDisabled(t *testing.T) {
	originalSSO := GlobalSSOAuth
	GlobalSSOAuth = nil
	t.Cleanup(func() { GlobalSSOAuth = originalSSO })

	tests := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
	}{
		{"login", HandleSSOLogIn, http.MethodGet, ssoLoginPath},
		{"acs", HandleSSOACS, http.MethodPost, SSOACSPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.handler(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandleAuthMethods(t *testing.T) {
	originalSSO := GlobalSSOAuth
	t.Cleanup(func() { GlobalSSOAuth = originalSSO })

	tests := []struct {
		name string
		sso  *SSOAuth
		want string
	}{
		{"password only", nil, `{"sso":false}`},
		{"sso enabled", &SSOAuth{}, `{"sso":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			GlobalSSOAuth = tt.sso
			rec := httptest.NewRecorder()
			HandleAuthMethods(rec, httptest.NewRequest(http.MethodGet, "/auth/methods", nil))
			if got := rec.Body.String(); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSafeRedirectPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"page", "/build/7", "/build/7"},
		{"page with query", "/jobs?q=a", "/jobs?q=a"},
		{"empty", "", "/"},
		{"relative path", "build/7", "/"},
		{"absolute URL", "https://evil.example.com/", "/"},
		{"scheme-relative URL", "//evil.example.com", "/"},
		{"triple slash", "///evil.example.com", "/"},
		{"backslash", "/\\evil.example.com", "/"},
		{"tab", "/\t/evil.example.com", "/"},
		{"newline", "/\n/evil.example.com", "/"},
		{"longest allowed", "/" + strings.Repeat("a", maxRedirectPathLength-1), "/" + strings.Repeat("a", maxRedirectPathLength-1)},
		{"too long", "/" + strings.Repeat("a", maxRedirectPathLength), "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeRedirectPath(tt.in); got != tt.want {
				t.Errorf("safeRedirectPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
