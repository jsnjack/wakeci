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
	"slices"
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

// errSSOGroup means the user signed in but is in none of the allowed groups
var errSSOGroup = errors.New("user is not in an allowed group")

// errSSOUnsolicited means the identity provider sent a login wakeci didn't
// ask for, e.g. when the user opens wakeci from the Google apps menu
var errSSOUnsolicited = errors.New("unsolicited SAML response")

// SSOAuth signs users in through a SAML 2.0 identity provider, such as Google
// Workspace, and lets in members of the allowed groups only
type SSOAuth struct {
	sp     *saml2.SAMLServiceProvider
	groups []string
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
// identity provider's metadata file
func NewSSOAuth(cfg SSOConfig) (*SSOAuth, error) {
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
		groups:       cfg.allowedGroups,
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
// provider, which must answer the login request requestID, and returns the
// user's NameID (their email in Google Workspace)
func (s *SSOAuth) VerifyResponse(encodedResponse, requestID string) (string, error) {
	// Checks the signature against the metadata certificates, the issuer,
	// destination, recipient, status and subject confirmation expiry
	info, err := s.sp.RetrieveAssertionInfo(encodedResponse)
	if err != nil {
		return "", err
	}
	switch {
	case info.WarningInfo.InvalidTime:
		return "", fmt.Errorf("assertion is expired or not valid yet")
	case info.WarningInfo.NotInAudience:
		return "", fmt.Errorf("assertion is for another service provider")
	case len(info.Assertions) != 1:
		return "", fmt.Errorf("response has %d assertions, want 1", len(info.Assertions))
	// gosaml2 only flags an audience restriction that doesn't match, so
	// require one. RetrieveAssertionInfo has checked that Conditions exists.
	case len(info.Assertions[0].Conditions.AudienceRestrictions) == 0:
		return "", fmt.Errorf("assertion has no audience restriction")
	}
	// Read InResponseTo from the signed assertion, not from the Response
	// element around it, which the identity provider may leave unsigned.
	// RetrieveAssertionInfo has checked that the subject confirmation exists.
	inResponseTo := info.Assertions[0].Subject.SubjectConfirmation.SubjectConfirmationData.InResponseTo
	switch {
	case inResponseTo == "":
		return "", errSSOUnsolicited
	case subtle.ConstantTimeCompare([]byte(inResponseTo), []byte(requestID)) != 1:
		return "", fmt.Errorf("response answers another login request")
	case !s.useRequest(requestID, time.Now()):
		return "", fmt.Errorf("login request %s was already answered", requestID)
	case info.NameID == "":
		return "", fmt.Errorf("assertion has no NameID")
	}
	groups := info.Values.GetAll(ssoGroupsAttribute)
	if !slices.ContainsFunc(groups, s.groupAllowed) {
		return "", fmt.Errorf("%w: %q is in groups %q", errSSOGroup, info.NameID, groups)
	}
	return info.NameID, nil
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

func (s *SSOAuth) groupAllowed(group string) bool {
	return slices.ContainsFunc(s.groups, func(allowed string) bool {
		return strings.EqualFold(strings.TrimSpace(group), allowed)
	})
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
