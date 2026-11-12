package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	saml2 "github.com/russellhaering/gosaml2"
	"github.com/russellhaering/gosaml2/types"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/sasha-s/go-deadlock"
)

// ssoGroupsAttribute is the SAML attribute that lists the user's groups. In
// Google Workspace it is the app attribute of the "Group membership" mapping.
const ssoGroupsAttribute = "groups"

// errSSOGroup means the user signed in but is in none of the groups that may
// log in
var errSSOGroup = errors.New("user is not in an allowed group")

// errSSOUnsolicited means the identity provider sent a login wakeci didn't
// ask for, e.g. when the user opens wakeci from the Google apps menu
var errSSOUnsolicited = errors.New("unsolicited SAML response")

// SSOAuth signs users in through a SAML 2.0 identity provider, such as Google
// Workspace, and lets in members of the groups that may log in only
type SSOAuth struct {
	sp *saml2.SAMLServiceProvider
	// certs sign the identity provider's responses
	certs []*x509.Certificate
	// admits reports whether someone in these groups may log in
	admits func(groups []string) bool
	// stateKey signs the login state cookie
	stateKey []byte
	// usedRequests holds the IDs of login requests that already got their
	// response, until their login state expires, so a response captured
	// together with its cookie can't be replayed
	usedMu       deadlock.Mutex
	usedRequests map[string]time.Time
}

// GlobalSSOAuth is the single sign-on client, nil when it isn't configured
var GlobalSSOAuth *SSOAuth

// NewSSOAuth creates a single sign-on client from config, reading the
// identity provider's metadata file. admits reports whether someone in the
// groups the identity provider sent may log in.
func NewSSOAuth(cfg SSOConfig, admits func(groups []string) bool) (*SSOAuth, error) {
	data, err := os.ReadFile(cfg.IDPMetadata)
	if err != nil {
		return nil, fmt.Errorf("read identity provider metadata: %w", err)
	}
	idp, err := parseIDPMetadata(data)
	if err != nil {
		return nil, fmt.Errorf("parse identity provider metadata %s: %w", cfg.IDPMetadata, err)
	}
	stateKey := make([]byte, 32)
	if _, err := rand.Read(stateKey); err != nil {
		return nil, fmt.Errorf("generate login state key: %w", err)
	}
	return &SSOAuth{
		sp: &saml2.SAMLServiceProvider{
			IdentityProviderSSOURL:      idp.ssoURL,
			IdentityProviderIssuer:      idp.entityID,
			IDPCertificateStore:         &dsig.MemoryX509CertificateStore{Roots: idp.certs},
			AssertionConsumerServiceURL: cfg.ACSURL,
			ServiceProviderIssuer:       cfg.EntityID,
			AudienceURI:                 cfg.EntityID,
			// A user in none of the mapped groups gets no attributes at all,
			// which should be reported as "not allowed", not as a broken response
			AllowMissingAttributes: true,
		},
		certs:        idp.certs,
		admits:       admits,
		stateKey:     stateKey,
		usedRequests: make(map[string]time.Time),
	}, nil
}

// idpMetadata is what wakeci needs from the identity provider's metadata
type idpMetadata struct {
	entityID string
	ssoURL   string
	certs    []*x509.Certificate
}

func parseIDPMetadata(data []byte) (*idpMetadata, error) {
	var desc types.EntityDescriptor
	if err := xml.Unmarshal(data, &desc); err != nil {
		return nil, err
	}
	if desc.EntityID == "" {
		return nil, fmt.Errorf("no entityID")
	}
	if desc.IDPSSODescriptor == nil {
		return nil, fmt.Errorf("no IDPSSODescriptor")
	}
	idp := &idpMetadata{entityID: desc.EntityID}
	for _, service := range desc.IDPSSODescriptor.SingleSignOnServices {
		if service.Binding == saml2.BindingHttpRedirect {
			idp.ssoURL = service.Location
			break
		}
	}
	if idp.ssoURL == "" {
		return nil, fmt.Errorf("no SingleSignOnService with the HTTP-Redirect binding")
	}
	for _, key := range desc.IDPSSODescriptor.KeyDescriptors {
		if key.Use != "" && key.Use != "signing" {
			continue
		}
		for _, cert := range key.KeyInfo.X509Data.X509Certificates {
			// Metadata files wrap and indent the base64
			der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(cert.Data), ""))
			if err != nil {
				return nil, fmt.Errorf("decode signing certificate: %w", err)
			}
			parsed, err := x509.ParseCertificate(der)
			if err != nil {
				return nil, fmt.Errorf("parse signing certificate: %w", err)
			}
			idp.certs = append(idp.certs, parsed)
		}
	}
	if len(idp.certs) == 0 {
		return nil, fmt.Errorf("no signing certificate")
	}
	return idp, nil
}

// AuthURL returns the identity provider URL that starts a login, and the ID
// of the login request that the response must answer
func (s *SSOAuth) AuthURL() (string, string, error) {
	doc, err := s.sp.BuildAuthRequestDocumentNoSig()
	if err != nil {
		return "", "", fmt.Errorf("build login request: %w", err)
	}
	requestID := doc.Root().SelectAttrValue("ID", "")
	authURL, err := s.sp.BuildAuthURLRedirect("", doc)
	if err != nil {
		return "", "", fmt.Errorf("build login URL: %w", err)
	}
	return authURL, requestID, nil
}

// VerifyResponse checks a base64 SAML response posted by the identity
// provider, which must answer the login request requestID, and returns who
// it logs in. A user in no allowed group gets an *ssoGroupError.
func (s *SSOAuth) VerifyResponse(encodedResponse, requestID string) (*ssoLogin, error) {
	// Checks the signature against the metadata certificates, the issuer,
	// destination, recipient, status and subject confirmation expiry
	info, err := s.sp.RetrieveAssertionInfo(encodedResponse)
	if err != nil {
		return nil, err
	}
	switch {
	case info.WarningInfo.InvalidTime:
		return nil, fmt.Errorf("assertion is expired or not valid yet")
	case info.WarningInfo.NotInAudience:
		return nil, fmt.Errorf("assertion is for another service provider")
	case len(info.Assertions) != 1:
		return nil, fmt.Errorf("response has %d assertions, want 1", len(info.Assertions))
	// gosaml2 only flags an audience restriction that doesn't match, so
	// require one. RetrieveAssertionInfo has checked that Conditions exists.
	case len(info.Assertions[0].Conditions.AudienceRestrictions) == 0:
		return nil, fmt.Errorf("assertion has no audience restriction")
	}
	// Read InResponseTo from the signed assertion, not from the Response
	// element around it, which the identity provider may leave unsigned.
	// RetrieveAssertionInfo has checked that the subject confirmation exists.
	inResponseTo := info.Assertions[0].Subject.SubjectConfirmation.SubjectConfirmationData.InResponseTo
	switch {
	case inResponseTo == "":
		return nil, errSSOUnsolicited
	case subtle.ConstantTimeCompare([]byte(inResponseTo), []byte(requestID)) != 1:
		return nil, fmt.Errorf("response answers another login request")
	case !s.useRequest(requestID, time.Now()):
		return nil, fmt.Errorf("login request %s was already answered", requestID)
	case info.NameID == "":
		return nil, fmt.Errorf("assertion has no NameID")
	}
	groups := []string{}
	for _, group := range info.Values.GetAll(ssoGroupsAttribute) {
		if group = strings.TrimSpace(group); group != "" {
			groups = append(groups, group)
		}
	}
	if !s.admits(groups) {
		return nil, &ssoGroupError{email: info.NameID, groups: groups}
	}
	return &ssoLogin{email: info.NameID, groups: groups, issuedAt: info.Assertions[0].IssueInstant}, nil
}

// ssoLogin is who a verified SAML response logs in
type ssoLogin struct {
	// email is the NameID, the email in Google Workspace
	email string
	// groups are those the identity provider sent, which for Google are
	// the mapped groups the user is in
	groups []string
	// issuedAt is when the signed assertion was issued
	issuedAt time.Time
}

// ssoGroupError means the user signed in but is in none of the allowed
// groups. It keeps who it was for the audit log.
type ssoGroupError struct {
	email  string
	groups []string
}

func (e *ssoGroupError) Error() string {
	return fmt.Sprintf("%s: %q is in groups %q", errSSOGroup, e.email, e.groups)
}

func (e *ssoGroupError) Unwrap() error {
	return errSSOGroup
}

// useRequest records that a login request got its response and reports
// whether it hadn't before. Only correctly signed responses that match a valid
// login state get here, so the map can't be flooded.
func (s *SSOAuth) useRequest(requestID string, now time.Time) bool {
	s.usedMu.Lock()
	defer s.usedMu.Unlock()
	for id, expires := range s.usedRequests {
		if !now.Before(expires) {
			delete(s.usedRequests, id)
		}
	}
	if _, used := s.usedRequests[requestID]; used {
		return false
	}
	// The login state was sealed before now, so it expires before this does
	s.usedRequests[requestID] = now.Add(ssoStateTTL)
	return true
}

// ssoLoginState is kept in a cookie from the redirect to the identity
// provider until its response, binding the response to the browser that
// started the login
type ssoLoginState struct {
	RequestID string `json:"id"`
	// Redirect is checked with safeRedirectPath before it is sealed
	Redirect string `json:"redirect"`
	Expires  int64  `json:"exp"`
}

// sealLoginState encodes and signs the login state. It is signed because the
// SAML response is a bearer token: with an unsigned cookie, whoever got hold
// of a response could replay it by writing its request ID into the cookie.
func (s *SSOAuth) sealLoginState(login ssoLoginState) (string, error) {
	payload, err := json.Marshal(login)
	if err != nil {
		return "", fmt.Errorf("marshal login state: %w", err)
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(payload) + "." + enc.EncodeToString(s.loginStateMAC(payload)), nil
}

// openLoginState checks the signature and expiry of a sealed login state
func (s *SSOAuth) openLoginState(value string, now time.Time) (*ssoLoginState, error) {
	encodedPayload, encodedMAC, ok := strings.Cut(value, ".")
	if !ok {
		return nil, fmt.Errorf("malformed login state")
	}
	enc := base64.RawURLEncoding
	payload, err := enc.DecodeString(encodedPayload)
	if err != nil {
		return nil, fmt.Errorf("decode login state: %w", err)
	}
	mac, err := enc.DecodeString(encodedMAC)
	if err != nil {
		return nil, fmt.Errorf("decode login state signature: %w", err)
	}
	if !hmac.Equal(mac, s.loginStateMAC(payload)) {
		return nil, fmt.Errorf("login state signature mismatch")
	}
	var login ssoLoginState
	if err := json.Unmarshal(payload, &login); err != nil {
		return nil, fmt.Errorf("parse login state: %w", err)
	}
	if login.RequestID == "" {
		return nil, fmt.Errorf("login state has no request ID")
	}
	if !now.Before(time.Unix(login.Expires, 0)) {
		return nil, fmt.Errorf("login state expired at %d", login.Expires)
	}
	return &login, nil
}

func (s *SSOAuth) loginStateMAC(payload []byte) []byte {
	mac := hmac.New(sha256.New, s.stateKey)
	mac.Write(payload)
	return mac.Sum(nil)
}
