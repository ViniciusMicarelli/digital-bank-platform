package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	notificationv1 "github.com/ViniciusMicarelli/digital-bank-platform/contracts/gen/go/bank/notification/v1"
)

func notificationJSON(n *notificationv1.Notification) map[string]any {
	return map[string]any{"id": n.GetId(), "account_id": n.GetAccountId(), "category": n.GetCategory(),
		"title": n.GetTitle(), "body": n.GetBody(), "channels": n.GetChannels(), "occurred_at": ts(n.GetOccurredAt()),
		"read": n.GetRead()}
}

// listNotifications: newest first; ?unread=true, ?before=<id>, ?limit=.
func (a *API) listNotifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	resp, err := a.c.Notifications.ListNotifications(r.Context(), &notificationv1.ListNotificationsRequest{
		CustomerId: customerID(r.Context()), UnreadOnly: q.Get("unread") == "true", BeforeId: q.Get("before"), Limit: int32(limit)})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	out := []map[string]any{}
	for _, n := range resp.GetNotifications() {
		out = append(out, notificationJSON(n))
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": out, "unread_count": resp.GetUnreadCount()})
}

// markNotificationsRead marks {ids}, or every unread one when ids is empty.
func (a *API) markNotificationsRead(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Notifications.MarkRead(r.Context(), &notificationv1.MarkReadRequest{CustomerId: customerID(r.Context()), Ids: req.IDs})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int32{"marked": resp.GetMarked()})
}

func (a *API) notificationPreferences(w http.ResponseWriter, r *http.Request) {
	resp, err := a.c.Notifications.GetPreferences(r.Context(), &notificationv1.GetPreferencesRequest{CustomerId: customerID(r.Context())})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"push": resp.GetPreferences().GetPush(), "email": resp.GetPreferences().GetEmail()})
}

func (a *API) updateNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Push  bool `json:"push"`
		Email bool `json:"email"`
	}
	if !decode(w, r, &req) {
		return
	}
	resp, err := a.c.Notifications.UpdatePreferences(r.Context(), &notificationv1.UpdatePreferencesRequest{
		CustomerId: customerID(r.Context()), Preferences: &notificationv1.Preferences{Push: req.Push, Email: req.Email}})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"push": resp.GetPreferences().GetPush(), "email": resp.GetPreferences().GetEmail()})
}

// streamLifetime bounds a stream so a client re-authenticates with a fresh
// token (EventSource reconnects by itself, sending Last-Event-ID).
const streamLifetime = 10 * time.Minute

// notificationStream sends new notifications as Server-Sent Events. A client
// reconnecting with Last-Event-ID gets what it missed.
func (a *API) notificationStream(w http.ResponseWriter, r *http.Request) {
	after := r.Header.Get("Last-Event-ID")
	if after != "" {
		if _, err := uuid.Parse(after); err != nil {
			writeError(w, r.Context(), http.StatusBadRequest, "INVALID_LAST_EVENT_ID", "Last-Event-ID must be a notification id")
			return
		}
	}
	rc := http.NewResponseController(w)
	lifetime := streamLifetime
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		lifetime = 25 * time.Second // end before the server's write timeout; the client reconnects
	}
	ctx, cancel := context.WithTimeout(r.Context(), lifetime)
	defer cancel()
	stream, err := a.c.Notifications.Subscribe(ctx, &notificationv1.SubscribeRequest{CustomerId: customerID(r.Context()), AfterId: after})
	if err != nil {
		writeGRPCError(w, r.Context(), err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no") // proxies must not buffer
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	_ = rc.Flush()

	type result struct {
		n   *notificationv1.Notification
		err error
	}
	recv := make(chan result)
	go func() {
		for {
			m, err := stream.Recv()
			select {
			case recv <- result{m.GetNotification(), err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
		case res := <-recv:
			if res.err != nil {
				return
			}
			data, _ := json.Marshal(notificationJSON(res.n))
			_, _ = fmt.Fprintf(w, "id: %s\nevent: notification\ndata: %s\n\n", res.n.GetId(), data)
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
