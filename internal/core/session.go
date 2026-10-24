package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

const secretService = "codeforces-client"

type Session struct {
	Protocol  int            `json:"protocol"`
	Handle    string         `json:"handle"`
	ExpiresAt int64          `json:"expiresAt"`
	Cookies   []*http.Cookie `json:"cookies"`
}
type SecretStore interface {
	Get() (Session, error)
	Set(Session) error
	Delete() error
}
type KeyringStore struct{}

func (KeyringStore) Get() (Session, error) {
	value, e := keyring.Get(secretService, "codeforces.com")
	if e == keyring.ErrNotFound {
		value, e = migrateStoredSession()
	}
	if e != nil {
		return Session{}, Fail("session_unavailable", "No stored session is available from the operating system credential store.")
	}
	return DecodeSession([]byte(value))
}
func (KeyringStore) Set(s Session) error {
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	if keyring.Set(secretService, "codeforces.com", string(b)) != nil {
		return Fail("secret_store_unavailable", "The operating system credential store is unavailable; no plaintext fallback is used.")
	}
	return nil
}
func (KeyringStore) Delete() error {
	if e := forgetPreviousSession(); e != nil {
		return Fail("secret_store_unavailable", "Cannot remove the previous development session.")
	}
	e := keyring.Delete(secretService, "codeforces.com")
	if e == keyring.ErrNotFound {
		return nil
	}
	if e != nil {
		return Fail("secret_store_unavailable", "Cannot remove the session from the operating system credential store.")
	}
	return nil
}
func DecodeSession(b []byte) (Session, error) {
	var s Session
	if len(b) > 64000 || json.Unmarshal(b, &s) != nil || s.Protocol != 1 || ValidHandle(s.Handle) != nil || len(s.Cookies) == 0 || len(s.Cookies) > 50 || s.ExpiresAt <= time.Now().Unix() || s.ExpiresAt > time.Now().Add(25*time.Hour).Unix() {
		return s, Fail("invalid_session", "Invalid or expired session data.")
	}
	for _, cookie := range s.Cookies {
		if cookie == nil || cookie.Valid() != nil || cookie.Name == "" || len(cookie.Value) > 16000 || strings.ContainsAny(cookie.Name+cookie.Value, "\r\n") || cookie.Domain != "" && cookie.Domain != "codeforces.com" && cookie.Domain != ".codeforces.com" {
			return Session{}, Fail("invalid_session", "Invalid session cookie.")
		}
	}
	return s, nil
}
func (c *Client) UseSession(s Session) error {
	b, _ := json.Marshal(s)
	if _, e := DecodeSession(b); e != nil {
		return e
	}
	u, _ := url.Parse(c.base)
	c.HTTP.Jar.SetCookies(u, s.Cookies)
	return nil
}
func (c *Client) Session(ctx context.Context) (Session, error) {
	p, e := c.Request(ctx, "GET", "/", nil)
	if e != nil {
		return Session{}, e
	}
	h, e := Handle(p.HTML)
	if e != nil {
		return Session{}, e
	}
	u, _ := url.Parse(c.base)
	return Session{Protocol: 1, Handle: h, ExpiresAt: time.Now().Add(24 * time.Hour).Unix(), Cookies: c.HTTP.Jar.Cookies(u)}, nil
}

// Login is experimental. Passwords are only held for this request and are never persisted.
func (c *Client) Login(ctx context.Context, username, password string) (Session, error) {
	if username == "" || password == "" || len(username) > 254 || len(password) > 4096 {
		return Session{}, Fail("invalid_argument", "A username and password are required on stdin.")
	}
	p, e := c.Request(ctx, "GET", "/enter", nil)
	if e != nil {
		return Session{}, e
	}
	csrf, e := CSRF(p.HTML)
	if e != nil {
		return Session{}, e
	}
	b := make([]byte, 16)
	if _, e = rand.Read(b); e != nil {
		return Session{}, e
	}
	p, e = c.Request(ctx, "POST", "/enter", url.Values{"csrf_token": {csrf}, "action": {"enter"}, "handleOrEmail": {username}, "password": {password}, "ftaa": {hex.EncodeToString(b)[:18]}, "bfaa": {"f1b3f18c715565b589b7823cda7448ce"}, "_tta": {"176"}, "remember": {"on"}})
	if e != nil {
		return Session{}, e
	}
	if _, e = Handle(p.HTML); e != nil {
		return Session{}, Fail("login_failed", "Login was not confirmed. The site may require verification, 2FA, or updated login fields. Use the official website.")
	}
	return c.Session(ctx)
}
func (c *Client) Languages(ctx context.Context, id int) ([]Language, error) {
	p, e := c.Request(ctx, "GET", "/contest/"+itoa(id)+"/submit", nil)
	if e != nil {
		return nil, e
	}
	if _, e = Handle(p.HTML); e != nil {
		return nil, e
	}
	return ParseLanguages(p.HTML)
}
