package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestHashAndCheckPassword(t *testing.T) {
	password := "supersecret"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "" {
		t.Fatalf("HashPassword returned empty hash")
	}

	ok, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash returned error: %v", err)
	}
	if !ok {
		t.Fatalf("CheckPasswordHash should return true for correct password")
	}

	ok, err = CheckPasswordHash("wrongpassword", hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash returned error for wrong password: %v", err)
	}
	if ok {
		t.Fatalf("CheckPasswordHash should return false for wrong password")
	}
}

func TestMakeJWT(t *testing.T) {
	secret := "mysecret"
	userID := uuid.New()

	jwtStr, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT returned error: %v", err)
	}
	if jwtStr == "" {
		t.Fatalf("MakeJWT returned empty token")
	}

	token, err := jwt.ParseWithClaims(jwtStr, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("Generated JWT failed to parse: %v", err)
	}

	claims := token.Claims.(*jwt.RegisteredClaims)
	if claims.Issuer != "chirpy" {
		t.Errorf("Expected issuer 'chirpy', got %s", claims.Issuer)
	}
	if claims.Subject != userID.String() {
		t.Errorf("Expected subject %s, got %s", userID.String(), claims.Subject)
	}
}

func TestValidateJWT_Success(t *testing.T) {
	secret := "mysecret"
	userID := uuid.New()

	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: userID.String(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	parsedID, err := ValidateJWT(tokenStr, secret)
	if err != nil {
		t.Fatalf("ValidateJWT returned error: %v", err)
	}

	if parsedID != userID {
		t.Fatalf("Expected %s, got %s", userID.String(), parsedID.String())
	}
}

func TestValidateJWT_InvalidSecret(t *testing.T) {
	userID := uuid.New()

	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: userID.String(),
	}).SignedString([]byte("WrongSecret"))
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	_, err = ValidateJWT(tokenStr, "ignored")
	if err == nil {
		t.Fatalf("ValidateJWT should fail when secret does not match hardcoded secret")
	}
}

func TestValidateJWT_InvalidToken(t *testing.T) {
	_, err := ValidateJWT("not-a-valid-jwt", "ignored")
	if err == nil {
		t.Fatalf("ValidateJWT should fail for invalid token string")
	}
}

func TestGetBearerToken(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer TOKEN_STRING")

	token, err := GetBearerToken(headers)
	if err != nil {
		t.Fatalf("Error when fetching token")
	}
	if token != "TOKEN_STRING" {
		t.Fatalf("ValidateJWT should fail for invalid token string")
	}
}
