package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const googleExchangeTimeout = 10 * time.Second

// Endpoints from https://accounts.google.com/.well-known/openid-configuration
var googleEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// Google issues ID tokens with either form of the issuer, see
// https://developers.google.com/identity/openid-connect/openid-connect#validatinganidtoken
var googleIssuers = []string{"https://accounts.google.com", "accounts.google.com"}

// errGoogleDomain means the account is a valid Google account outside the
// configured Workspace domain
var errGoogleDomain = errors.New("account is not in the allowed Google Workspace domain")

// GoogleAuth signs users in with Google OpenID Connect (authorization code
// flow with PKCE), restricted to a single Google Workspace domain.
type GoogleAuth struct {
	oauth  *oauth2.Config
	domain string
	client *http.Client
}

// GlobalGoogleAuth is the Google sign-in client, nil when it isn't configured
var GlobalGoogleAuth *GoogleAuth

// NewGoogleAuth creates a Google sign-in client from config
func NewGoogleAuth(cfg GoogleConfig) *GoogleAuth {
	return &GoogleAuth{
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     googleEndpoint,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       []string{"openid", "email"},
		},
		domain: cfg.Domain,
		client: &http.Client{Timeout: googleExchangeTimeout},
	}
}

// AuthCodeURL returns the Google sign-in URL for one login attempt
func (g *GoogleAuth) AuthCodeURL(state, nonce, verifier string) string {
	return g.oauth.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("nonce", nonce),
		// hd only narrows Google's account chooser; the hd claim in the ID
		// token is what's enforced
		oauth2.SetAuthURLParam("hd", g.domain),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)
}

// Exchange trades an authorization code for an ID token and returns the
// email of the signed-in Workspace account
func (g *GoogleAuth) Exchange(ctx context.Context, code, verifier, nonce string) (string, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, g.client)
	token, err := g.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", fmt.Errorf("exchange code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", fmt.Errorf("token response has no id_token")
	}
	return g.verifyIDToken(rawIDToken, nonce, time.Now())
}

type googleIDTokenClaims struct {
	Issuer        string `json:"iss"`
	Audience      string `json:"aud"`
	Expiry        int64  `json:"exp"`
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	HostedDomain  string `json:"hd"`
}

// verifyIDToken checks the claims of an ID token received directly from
// Google's token endpoint. The signature is not checked: the token arrived
// over TLS from Google in exchange for the client secret, which OpenID
// Connect Core 3.1.3.7 accepts in place of signature validation. Don't reuse
// this for ID tokens that come from anywhere else.
func (g *GoogleAuth) verifyIDToken(rawIDToken, nonce string, now time.Time) (string, error) {
	parts := strings.Split(rawIDToken, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed id_token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode id_token payload: %w", err)
	}
	var claims googleIDTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse id_token claims: %w", err)
	}
	switch {
	case !slices.Contains(googleIssuers, claims.Issuer):
		return "", fmt.Errorf("unexpected issuer %q", claims.Issuer)
	case claims.Audience != g.oauth.ClientID:
		return "", fmt.Errorf("unexpected audience %q", claims.Audience)
	case !now.Before(time.Unix(claims.Expiry, 0)):
		return "", fmt.Errorf("id_token expired at %d", claims.Expiry)
	case nonce == "" || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1:
		return "", fmt.Errorf("nonce mismatch")
	case claims.Email == "":
		return "", fmt.Errorf("id_token has no email")
	case !claims.EmailVerified:
		return "", fmt.Errorf("email %q is not verified", claims.Email)
	case !strings.EqualFold(claims.HostedDomain, g.domain):
		return "", fmt.Errorf("%w: email %q, hd %q", errGoogleDomain, claims.Email, claims.HostedDomain)
	}
	return claims.Email, nil
}
