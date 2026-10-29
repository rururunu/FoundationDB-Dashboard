// Package auth checks the optional dashboard sign-in and remembers sessions.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const (
	CookieName = "fdb_session"
	sessionTTL = 12 * time.Hour
	maxFails   = 8
	lockFor    = 30 * time.Second
)

var (
	// ErrCredentials is a wrong username or password.
	ErrCredentials = errors.New("用户名或密码不正确")
	// ErrLocked means this client failed too many times.
	ErrLocked = errors.New("尝试次数过多，请稍后再试")
)

// Gate is the sign-in check for one process. The password is kept only as a hash.
type Gate struct {
	userSum [32]byte
	passSum [32]byte

	mu       sync.Mutex
	sessions map[string]time.Time
	fails    map[string]int
	locked   map[string]time.Time
}

// New remembers the username and password for later checks.
func New(user, password string) *Gate {
	return &Gate{
		userSum:  sha256.Sum256([]byte(user)),
		passSum:  sha256.Sum256([]byte(password)),
		sessions: map[string]time.Time{},
		fails:    map[string]int{},
		locked:   map[string]time.Time{},
	}
}

// Login checks the pair for this client address and returns a session token.
func (g *Gate) Login(ip, user, password string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if until, ok := g.locked[ip]; ok {
		if now.Before(until) {
			return "", ErrLocked
		}
		delete(g.locked, ip)
		g.fails[ip] = 0
	}
	userSum := sha256.Sum256([]byte(user))
	passSum := sha256.Sum256([]byte(password))
	userOK := subtle.ConstantTimeCompare(userSum[:], g.userSum[:]) == 1
	passOK := subtle.ConstantTimeCompare(passSum[:], g.passSum[:]) == 1
	if userOK && passOK {
		delete(g.fails, ip)
		token, err := newToken()
		if err != nil {
			return "", err
		}
		g.sessions[token] = now.Add(sessionTTL)
		return token, nil
	}
	g.fails[ip]++
	if g.fails[ip] >= maxFails {
		g.locked[ip] = now.Add(lockFor)
		g.fails[ip] = 0
		return "", ErrLocked
	}
	return "", ErrCredentials
}

// Valid reports whether the token is a live session. Expired tokens are dropped.
func (g *Gate) Valid(token string) bool {
	if token == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(g.sessions, token)
		return false
	}
	return true
}

// Refresh extends a live session.
func (g *Gate) Refresh(token string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.sessions[token]
	if !ok || time.Now().After(until) {
		delete(g.sessions, token)
		return false
	}
	g.sessions[token] = time.Now().Add(sessionTTL)
	return true
}

// Revoke drops one session.
func (g *Gate) Revoke(token string) {
	if token == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.sessions, token)
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
