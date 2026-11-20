package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// MaxUserAgent bounds what is kept of a client's User-Agent.
const MaxUserAgent = 300

// Device is where a login comes from, as the gateway saw it.
type Device struct {
	UserAgent string
	IP        string
}

// Session is one login: the family of its refresh tokens. It is active
// until revoked or until its refresh token expires (RefreshTokenTTL after
// it was last used).
type Session struct {
	ID         uuid.UUID // the refresh token family
	CustomerID uuid.UUID
	Device     string // readable, from the User-Agent
	UserAgent  string
	IP         string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// NewSession opens a session for a login from d.
func NewSession(customer uuid.UUID, d Device, now time.Time) Session {
	ua := strings.TrimSpace(d.UserAgent)
	if len(ua) > MaxUserAgent {
		ua = ua[:MaxUserAgent]
	}
	return Session{ID: uuid.Must(uuid.NewV7()), CustomerID: customer, Device: DeviceLabel(ua), UserAgent: ua, IP: d.IP,
		CreatedAt: now, LastUsedAt: now}
}

// DeviceLabel names a User-Agent the way a customer recognizes it, e.g.
// "Chrome no macOS" or "App no iPhone".
func DeviceLabel(ua string) string {
	has := func(s ...string) bool {
		for _, x := range s {
			if strings.Contains(ua, x) {
				return true
			}
		}
		return false
	}
	var os string
	switch {
	case has("iPhone"):
		os = "iPhone"
	case has("iPad"):
		os = "iPad"
	case has("Android", "okhttp"):
		os = "Android"
	case has("CFNetwork", "Darwin") && !has("Macintosh"):
		os = "iPhone" // the native app on iOS
	case has("Macintosh", "Mac OS X"):
		os = "macOS"
	case has("Windows"):
		os = "Windows"
	case has("CrOS"):
		os = "ChromeOS"
	case has("Linux"):
		os = "Linux"
	}
	var client string
	switch {
	case has("CFNetwork", "okhttp", "Expo", "digital-bank-mobile"):
		client = "App"
	case has("Edg/"):
		client = "Edge"
	case has("OPR/", "Opera"):
		client = "Opera"
	case has("Firefox/", "FxiOS"):
		client = "Firefox"
	case has("Chrome/", "CriOS"):
		client = "Chrome"
	case has("Safari/"):
		client = "Safari"
	}
	switch {
	case client != "" && os != "":
		return client + " no " + os
	case client != "":
		return client
	case os != "":
		return "Navegador no " + os
	}
	return "Dispositivo desconhecido"
}
