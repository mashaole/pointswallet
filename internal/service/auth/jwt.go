package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	JTI  string `json:"jti"`
	Exp  int64  `json:"exp"`
}

func SignToken(secret string, claims Claims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := header + "." + payloadEnc
	sig := signHS256(signingInput, secret)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func VerifyToken(secret, token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New("invalid token format")
	}
	signingInput := parts[0] + "." + parts[1]
	expected := signHS256(signingInput, secret)
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, errors.New("invalid token signature encoding")
	}
	if !hmac.Equal(expected, got) {
		return Claims{}, errors.New("invalid token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, errors.New("invalid token payload")
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, err
	}
	if time.Now().Unix() > claims.Exp {
		return Claims{}, errors.New("token expired")
	}
	return claims, nil
}

func signHS256(input, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(input))
	return mac.Sum(nil)
}

func NewClaims(accountID, role, jti string, ttl time.Duration) Claims {
	return Claims{
		Sub:  accountID,
		Role: role,
		JTI:  jti,
		Exp:  time.Now().Add(ttl).Unix(),
	}
}

func ParseBearer(authHeader string) (string, error) {
	if authHeader == "" {
		return "", fmt.Errorf("missing authorization header")
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", fmt.Errorf("invalid authorization header")
	}
	return strings.TrimSpace(strings.TrimPrefix(authHeader, prefix)), nil
}
