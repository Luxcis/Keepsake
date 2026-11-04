package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse 🐎")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, hashPrefix+"$") {
		t.Fatalf("unexpected format: %s", hash)
	}
	if err := VerifyPassword("correct horse 🐎", hash); err != nil {
		t.Fatalf("verify ok password: %v", err)
	}
	if err := VerifyPassword("wrong", hash); err == nil {
		t.Fatal("verify wrong password should fail")
	} else if err != ErrMismatch {
		t.Fatalf("expect ErrMismatch, got %v", err)
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "not-a-phc", "argon2id$v=19$AA$AA$AA"} {
		if err := VerifyPassword("x", bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestSaltUniquePerHash(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("hashes of same password must differ (unique salt)")
	}
}
