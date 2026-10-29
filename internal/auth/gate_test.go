package auth

import (
	"errors"
	"testing"
	"time"
)

func TestLoginAndSession(t *testing.T) {
	g := New("admin", "secret")
	if _, err := g.Login("10.0.0.1", "admin", "nope"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := g.Login("10.0.0.1", "other", "secret"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("wrong user: %v", err)
	}
	token, err := g.Login("10.0.0.1", "admin", "secret")
	if err != nil || token == "" {
		t.Fatal(err)
	}
	if !g.Valid(token) || !g.Refresh(token) {
		t.Fatal("session should stay valid")
	}
	g.Revoke(token)
	if g.Valid(token) {
		t.Fatal("revoked session should be gone")
	}
}

func TestLoginLocksAfterRepeatedFailures(t *testing.T) {
	g := New("admin", "secret")
	var err error
	for i := 0; i < maxFails; i++ {
		_, err = g.Login("10.0.0.8", "admin", "nope")
	}
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("last failure: %v", err)
	}
	if _, err = g.Login("10.0.0.8", "admin", "secret"); !errors.Is(err, ErrLocked) {
		t.Fatalf("locked client accepted a good password: %v", err)
	}
	g.mu.Lock()
	g.locked["10.0.0.8"] = time.Now().Add(-time.Second)
	g.mu.Unlock()
	if _, err = g.Login("10.0.0.8", "admin", "secret"); err != nil {
		t.Fatal(err)
	}
}
