package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/oauth2"
)

// Offboarding works without anyone touching wakeci: every Google user who
// signs in leaves a refresh token behind, and the checker redeems it every
// googleAccountCheckInterval. Once Google refuses it (account deleted,
// suspended or app access revoked) or the account leaves the allowed
// domains, the account and its API keys are deleted and its sessions ended.
const (
	googleAccountCheckInterval = 5 * time.Minute
	// googleAccountMaxUnverified fails closed: when Google hasn't confirmed
	// an account for this long (e.g. it's unreachable), its keys and
	// sessions are refused until a check succeeds again
	googleAccountMaxUnverified = 30 * time.Minute
	googleAccountCheckTimeout  = 10 * time.Second
)

var errGoogleAccountInactive = errors.New("google account is not active")

// googleUserInfoURL is Google's OpenID Connect userinfo endpoint; a variable
// so tests can point it at a fake Google
var googleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"

// googleAccount is a Google user whose access Google still vouches for
type googleAccount struct {
	// ID is the owner's Principal.ID: "google:<sub>"
	ID     string `json:"id"`
	Email  string `json:"email"`
	Domain string `json:"domain"`
	// RefreshToken only carries the openid and email scopes, so it can't
	// read anything from the Google account; wakeci only uses it to ask
	// Google whether the account is still active.
	RefreshToken string    `json:"refreshToken"`
	VerifiedAt   time.Time `json:"verifiedAt"`
	// LoginID is new for every stored login. The checker only acts on what
	// it learned from Google if it's unchanged: a login since makes it stale,
	// even one that reused the refresh token.
	LoginID string `json:"loginID"`
}

func getGoogleAccount(id string) (*googleAccount, error) {
	var account *googleAccount
	err := DB.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(GoogleAccountsBucket).Get([]byte(id))
		if v == nil {
			return nil
		}
		account = &googleAccount{}
		return json.Unmarshal(v, account)
	})
	if err != nil {
		return nil, fmt.Errorf("read google account %s: %w", id, err)
	}
	return account, nil
}

// putGoogleAccount stores a login of account, under a new LoginID
func putGoogleAccount(account *googleAccount) error {
	loginID, err := randomHex(16)
	if err != nil {
		return fmt.Errorf("generate login id: %w", err)
	}
	account.LoginID = loginID
	data, err := json.Marshal(account)
	if err != nil {
		return fmt.Errorf("marshal google account: %w", err)
	}
	return DB.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(GoogleAccountsBucket).Put([]byte(account.ID), data)
	})
}

// checkGoogleAccount returns an error wrapping errGoogleAccountInactive
// unless Google has recently confirmed account id and its domain is still
// allowed
func checkGoogleAccount(id string, now time.Time) error {
	account, err := getGoogleAccount(id)
	if err != nil {
		return err
	}
	switch {
	case account == nil:
		return fmt.Errorf("%w: %s was offboarded or never signed in", errGoogleAccountInactive, id)
	case !Config.Google.Allows(account.Domain):
		return fmt.Errorf("%w: domain %q of %s is not allowed", errGoogleAccountInactive, account.Domain, account.Email)
	case now.Sub(account.VerifiedAt) > googleAccountMaxUnverified:
		return fmt.Errorf("%w: %s not confirmed by Google since %s", errGoogleAccountInactive, account.Email, account.VerifiedAt)
	}
	return nil
}

// offboardGoogleAccount removes everything that lets checked in: the account
// itself, its API keys and its sessions, in one transaction, so nothing can
// be added in between (CreateAPIKey checks in its own). checked is the
// account as it was when Google was asked about it; if a login has been
// stored since, Google's verdict is stale, and nothing is removed.
func offboardGoogleAccount(logger *slog.Logger, checked googleAccount, reason string) error {
	id := checked.ID
	var email string
	revoked := 0
	stale := false
	sessions := 0
	err := DB.Update(func(tx *bolt.Tx) error {
		accounts := tx.Bucket(GoogleAccountsBucket)
		if v := accounts.Get([]byte(id)); v != nil {
			var account googleAccount
			if err := json.Unmarshal(v, &account); err != nil {
				return fmt.Errorf("unmarshal google account: %w", err)
			}
			if account.LoginID != checked.LoginID {
				stale = true
				return nil
			}
			email = account.Email
		}
		// A login stores the account before creating its session, and has to
		// wait for this transaction to do so: later sessions are newer logins'
		cutoff := time.Now()
		if err := accounts.Delete([]byte(id)); err != nil {
			return err
		}

		var err error
		revoked, err = deleteOwnerKeys(tx, id)
		if err != nil {
			return err
		}
		sessions = GlobalSessionStorage.DeletePrincipal(id, cutoff)
		return nil
	})
	if err != nil {
		return fmt.Errorf("offboard google account %s: %w", id, err)
	}
	if stale {
		logger.Info("google account signed in again during its check, keeping it", "id", id)
		return nil
	}
	WSHub.Recheck()
	logger.Warn("google account offboarded", "id", id, "email", email, "reason", reason, "keysRevoked", revoked, "sessionsEnded", sessions)
	return nil
}

// verifyGoogleAccount asks Google whether account is still active by
// redeeming its refresh token, and offboards it if not
func verifyGoogleAccount(ctx context.Context, logger *slog.Logger, account googleAccount, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, googleAccountCheckTimeout)
	defer cancel()
	token, err := googleOAuthConfigFor("").TokenSource(ctx, &oauth2.Token{RefreshToken: account.RefreshToken}).Token()
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant" {
		return offboardGoogleAccount(logger, account, "google rejected the refresh token: "+retrieveErr.ErrorDescription)
	}
	if err != nil {
		// Not a verdict on the account; googleAccountMaxUnverified bounds
		// how long this can go on before access is refused anyway
		return fmt.Errorf("refresh google token of %s: %w", account.Email, err)
	}

	// The account is only re-confirmed with Google's current claims: an
	// earlier domain must never be carried forward
	var claims *googleIDClaims
	if raw, ok := token.Extra("id_token").(string); ok && raw != "" {
		claims, err = parseGoogleIDToken(raw, Config.Google.ClientID, now)
		if err != nil {
			if errors.Is(err, errGoogleEmailUnverified) {
				return offboardGoogleAccount(logger, account, "unverified email")
			}
			return fmt.Errorf("validate refreshed google id token: %w", err)
		}
	} else {
		// Google may leave the ID token out of a refresh response
		claims, err = fetchGoogleUserInfo(ctx, token.AccessToken)
		if err != nil {
			return fmt.Errorf("fetch current claims of %s: %w", account.Email, err)
		}
		if claims.Email == "" || !claims.EmailVerified {
			return offboardGoogleAccount(logger, account, fmt.Sprintf("email %q is not verified", claims.Email))
		}
	}
	if ProviderGoogle+":"+claims.Subject != account.ID {
		return offboardGoogleAccount(logger, account, "claims belong to another account")
	}
	email, domain := strings.ToLower(claims.Email), strings.ToLower(claims.HostedDomain)
	if !Config.Google.Allows(domain) {
		return offboardGoogleAccount(logger, account, fmt.Sprintf("domain %q is not allowed", domain))
	}

	return DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(GoogleAccountsBucket)
		v := b.Get([]byte(account.ID))
		if v == nil {
			return nil
		}
		var current googleAccount
		if err := json.Unmarshal(v, &current); err != nil {
			return fmt.Errorf("unmarshal google account: %w", err)
		}
		if current.LoginID != account.LoginID {
			// A login stored fresher data meanwhile
			return nil
		}
		current.Email, current.Domain, current.VerifiedAt = email, domain, now
		if token.RefreshToken != "" && token.RefreshToken != account.RefreshToken {
			current.RefreshToken = token.RefreshToken
		}
		data, err := json.Marshal(current)
		if err != nil {
			return fmt.Errorf("marshal google account: %w", err)
		}
		return b.Put([]byte(account.ID), data)
	})
}

// fetchGoogleUserInfo reads the account's current claims from Google's
// userinfo endpoint, authenticated with a freshly refreshed access token
func fetchGoogleUserInfo(ctx context.Context, accessToken string) (*googleIDClaims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request userinfo: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			L.Log(ctx, LevelTrace, "close userinfo response", "err", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo returned %s", resp.Status)
	}
	var claims googleIDClaims
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&claims); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}
	if claims.Subject == "" {
		return nil, errors.New("userinfo has no subject")
	}
	return &claims, nil
}

// verifyGoogleAccounts checks every stored Google account once
func verifyGoogleAccounts(ctx context.Context, logger *slog.Logger, now time.Time) {
	var accounts []googleAccount
	err := DB.View(func(tx *bolt.Tx) error {
		return tx.Bucket(GoogleAccountsBucket).ForEach(func(_, v []byte) error {
			var account googleAccount
			if err := json.Unmarshal(v, &account); err != nil {
				return fmt.Errorf("unmarshal google account: %w", err)
			}
			accounts = append(accounts, account)
			return nil
		})
	})
	if err != nil {
		logger.Error("list google accounts", "err", err)
		return
	}
	for _, account := range accounts {
		if err := verifyGoogleAccount(ctx, logger, account, now); err != nil {
			logger.Warn("verify google account", "id", account.ID, "err", err)
		}
	}
}

// RunGoogleAccountChecker verifies every Google account right away, so a
// restart after downtime doesn't leave keys refused for a full interval,
// then every googleAccountCheckInterval until ctx is done
func RunGoogleAccountChecker(ctx context.Context) {
	logger := L.With("component", "google-accounts")
	ticker := time.NewTicker(googleAccountCheckInterval)
	defer ticker.Stop()
	for {
		verifyGoogleAccounts(ctx, logger, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// newGoogleSession cannot publish a session after its login was offboarded.
func newGoogleSession(r *http.Request, principal *Principal, login *googleAccount) (*http.Cookie, error) {
	var cookie *http.Cookie
	err := DB.Update(func(tx *bolt.Tx) error {
		raw := tx.Bucket(GoogleAccountsBucket).Get([]byte(login.ID))
		if raw == nil {
			return errGoogleAccountInactive
		}
		var current googleAccount
		if err := json.Unmarshal(raw, &current); err != nil {
			return fmt.Errorf("read session login: %w", err)
		}
		if current.LoginID != login.LoginID {
			return errGoogleAccountInactive
		}
		var err error
		cookie, err = GlobalSessionStorage.New(r, principal)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("create google session: %w", err)
	}
	return cookie, nil
}
