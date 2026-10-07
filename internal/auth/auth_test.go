package auth

import (
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("correct horse battery staple", h)
	if err != nil || !ok {
		t.Fatalf("expected match, got ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong password", h)
	if err != nil || ok {
		t.Fatalf("expected mismatch, got ok=%v err=%v", ok, err)
	}
}

func TestHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("same password here")
	b, _ := HashPassword("same password here")
	if a == b {
		t.Fatal("expected different hashes for the same password")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(time.Hour, 2)
	if !l.Allow("ip") || !l.Allow("ip") {
		t.Fatal("expected burst of 2 to be allowed")
	}
	if l.Allow("ip") {
		t.Fatal("expected third attempt to be limited")
	}
	if !l.Allow("other-ip") {
		t.Fatal("expected a different key to be allowed")
	}
}
