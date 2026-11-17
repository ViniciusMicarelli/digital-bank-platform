//go:build e2e

package e2e

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// totp computes an RFC 6238 code, like an authenticator app.
func totp(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[off:off+4])&0x7fffffff)%1_000_000)
}

func TestAuthFlow(t *testing.T) {
	c := &client{t: t, base: addr("GATEWAY_URL", "http://127.0.0.1:8000")}
	email := "carla." + uuid.NewString()[:8] + "@example.com"

	if status, _ := c.call("POST", "/v1/auth/register", map[string]string{"email": email, "password": "short", "full_name": "Carla"}, nil); status != 400 {
		t.Fatalf("weak password accepted: %d", status)
	}
	if status, _ := c.call("POST", "/v1/auth/register", map[string]string{"email": email, "password": e2ePassword, "full_name": "Carla Dias"}, nil); status != 201 {
		t.Fatalf("register: %d", status)
	}
	if status, body := c.call("POST", "/v1/auth/login", map[string]string{"email": email, "password": "wrong password!"}, nil); status != 401 || body["error"].(map[string]any)["code"] != "INVALID_CREDENTIALS" {
		t.Fatalf("wrong password: %d %v", status, body)
	}
	_, session := c.call("POST", "/v1/auth/login", map[string]string{"email": email, "password": e2ePassword}, nil)
	c.token = session["access_token"].(string)
	if status, _ := c.call("GET", "/v1/accounts", nil, nil); status != 200 {
		t.Fatalf("token from identity-service rejected by the gateway: %d", status)
	}

	t.Run("refresh rotates and detects reuse", func(t *testing.T) {
		first := session["refresh_token"].(string)
		status, next := c.call("POST", "/v1/auth/refresh", map[string]string{"refresh_token": first}, nil)
		if status != 200 || next["refresh_token"] == first {
			t.Fatalf("refresh: %d %v", status, next)
		}
		if status, body := c.call("POST", "/v1/auth/refresh", map[string]string{"refresh_token": first}, nil); status != 401 || body["error"].(map[string]any)["code"] != "REFRESH_TOKEN_REUSED" {
			t.Fatalf("reuse: %d %v", status, body)
		}
		if status, _ := c.call("POST", "/v1/auth/refresh", map[string]string{"refresh_token": next["refresh_token"].(string)}, nil); status != 401 {
			t.Fatalf("session must be revoked after reuse: %d", status)
		}
	})

	t.Run("TOTP", func(t *testing.T) {
		status, enroll := c.call("POST", "/v1/auth/mfa/totp", nil, nil)
		if status != 200 {
			t.Fatalf("enroll: %d %v", status, enroll)
		}
		secret := enroll["secret"].(string)
		now := time.Now()
		if status, body := c.call("POST", "/v1/auth/mfa/totp/confirm", map[string]string{"code": totp(t, secret, now)}, nil); status != 204 {
			t.Fatalf("confirm: %d %v", status, body)
		}
		if status, body := c.call("POST", "/v1/auth/login", map[string]string{"email": email, "password": e2ePassword}, nil); status != 401 || body["error"].(map[string]any)["code"] != "MFA_REQUIRED" {
			t.Fatalf("login without code: %d %v", status, body)
		}
		// The confirmation code is spent; the next step's code works (±1 step is accepted).
		status, body := c.call("POST", "/v1/auth/login", map[string]string{"email": email, "password": e2ePassword, "totp_code": totp(t, secret, now.Add(30*time.Second))}, nil)
		if status != 200 {
			t.Fatalf("login with code: %d %v", status, body)
		}
		if status, _ := c.call("POST", "/v1/auth/logout", map[string]string{"refresh_token": body["refresh_token"].(string)}, nil); status != 204 {
			t.Fatalf("logout: %d", status)
		}
	})

	c.token = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.forged"
	if status, _ := c.call("GET", "/v1/accounts", nil, nil); status != 401 {
		t.Fatalf("forged token: %d", status)
	}
}
