package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMakeAndValidateJWT_Success(t *testing.T) {
	secret := "supersecretkey"
	userID := uuid.New()
	token, err := MakeJWT(userID, secret, time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT failed: %v", err)
	}
	parsedID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT failed: %v", err)
	}
	if parsedID != userID {
		t.Errorf("Expected userID %v, got %v", userID, parsedID)
	}
}

func TestValidateJWT_Expired(t *testing.T) {
	secret := "supersecretkey"
	userID := uuid.New()
	token, err := MakeJWT(userID, secret, -time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT failed: %v", err)
	}
	_, err = ValidateJWT(token, secret)
	if err == nil {
		t.Error("Expected error for expired token, got nil")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	secret := "supersecretkey"
	wrongSecret := "wrongkey"
	userID := uuid.New()
	token, err := MakeJWT(userID, secret, time.Minute)
	if err != nil {
		t.Fatalf("MakeJWT failed: %v", err)
	}
	_, err = ValidateJWT(token, wrongSecret)
	if err == nil {
		t.Error("Expected error for wrong secret, got nil")
	}
}
