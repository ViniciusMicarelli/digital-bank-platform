//go:build e2e

package e2e

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// sse opens the live notification stream and returns its event lines.
func sse(t *testing.T, c *client, lastEventID string) (<-chan string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", c.base+"/v1/notifications/stream", nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	lines := make(chan string, 100)
	go func() {
		defer close(lines)
		s := bufio.NewScanner(resp.Body)
		for s.Scan() {
			lines <- s.Text()
		}
	}()
	return lines, func() { cancel(); resp.Body.Close() }
}

func waitFor(t *testing.T, lines <-chan string, prefix, contains string) string {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("stream closed waiting for %q", contains)
			}
			if strings.HasPrefix(l, prefix) && strings.Contains(l, contains) {
				return l
			}
		case <-deadline:
			t.Fatalf("no %q on the stream", contains)
		}
	}
}

func TestNotifications(t *testing.T) {
	ana := login(t, "ana-notif-"+uuid.NewString()[:8])
	_, acc := ana.call("POST", "/v1/accounts", map[string]string{"holder_name": "Ana Souza"}, idem())
	accID := acc["id"].(string)
	key := "ana." + uuid.NewString()[:8] + "@example.com"
	ana.call("POST", "/v1/accounts/"+accID+"/pix/keys", map[string]string{"type": "EMAIL", "value": key}, nil)

	if _, p := ana.call("GET", "/v1/notifications/preferences", nil, nil); p["push"] != true || p["email"] != false {
		t.Fatalf("default preferences = %v", p)
	}
	if _, p := ana.call("PUT", "/v1/notifications/preferences", map[string]bool{"push": true, "email": true}, nil); p["email"] != true {
		t.Fatalf("update preferences = %v", p)
	}

	lines, stop := sse(t, ana, "")
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "75.00"}, nil)
	data := waitFor(t, lines, "data: ", "Pix recebido")
	if !strings.Contains(data, "R$ 75,00") {
		t.Fatalf("streamed %s", data)
	}
	stop()

	// Missed while disconnected: replayed from Last-Event-ID.
	_, list := ana.call("GET", "/v1/notifications", nil, nil)
	last := list["notifications"].([]any)[0].(map[string]any)
	ana.call("POST", "/v1/sandbox/pix-in", map[string]string{"key": key, "amount": "1.50"}, nil)
	time.Sleep(time.Second)
	lines, stop = sse(t, ana, last["id"].(string))
	waitFor(t, lines, "data: ", "R$ 1,50")
	stop()

	_, list = ana.call("GET", "/v1/notifications?unread=true", nil, nil)
	if list["unread_count"].(float64) < 2 {
		t.Fatalf("notifications = %v", list)
	}
	if _, m := ana.call("POST", "/v1/notifications/read", map[string]any{}, nil); m["marked"].(float64) < 2 {
		t.Fatalf("mark all read = %v", m)
	}
	if _, list = ana.call("GET", "/v1/notifications", nil, nil); list["unread_count"].(float64) != 0 {
		t.Fatalf("after reading = %v", list)
	}

	// Each customer only sees their own.
	bruno := login(t, "bruno-notif-"+uuid.NewString()[:8])
	if _, l := bruno.call("GET", "/v1/notifications", nil, nil); len(l["notifications"].([]any)) != 0 {
		t.Fatalf("bruno sees = %v", l)
	}
}
