// Package domain holds the back-office rules: the audit chain and four-eyes
// approvals.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
)

// RefusedError is a business refusal (FAILED_PRECONDITION).
type RefusedError struct{ Reason, Msg string }

func (e *RefusedError) Error() string { return e.Reason + ": " + e.Msg }

func Refuse(reason, msg string) error { return &RefusedError{Reason: reason, Msg: msg} }

// AsRefused unwraps a refusal.
func AsRefused(err error) (*RefusedError, bool) {
	var r *RefusedError
	ok := errors.As(err, &r)
	return r, ok
}

func invalid(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidArgument, msg) }

// GenesisHash is the previous hash of the first event.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Event is one staff action in the audit trail.
type Event struct {
	Seq        int64
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Details    map[string]string
	At         time.Time
	PrevHash   string
	Hash       string
}

// Validate checks what every event needs.
func (e Event) Validate() error {
	switch {
	case strings.TrimSpace(e.ActorID) == "":
		return invalid("actor_id is required")
	case strings.TrimSpace(e.Action) == "":
		return invalid("action is required")
	}
	return nil
}

// ComputeHash chains the event to the previous one: SHA-256 over the
// previous hash and every field, each length-prefixed so no two different
// events encode the same way. Details are sorted by key; the time is UTC
// with microsecond precision (what PostgreSQL keeps).
func (e Event) ComputeHash() string {
	h := sha256.New()
	field := func(s string) {
		h.Write([]byte(strconv.Itoa(len(s))))
		h.Write([]byte{':'})
		h.Write([]byte(s))
	}
	field(e.PrevHash)
	field(strconv.FormatInt(e.Seq, 10))
	field(e.ActorID)
	field(e.Action)
	field(e.TargetType)
	field(e.TargetID)
	keys := make([]string, 0, len(e.Details))
	for k := range e.Details {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	field(strconv.Itoa(len(keys)))
	for _, k := range keys {
		field(k)
		field(e.Details[k])
	}
	field(e.At.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano))
	return hex.EncodeToString(h.Sum(nil))
}

// Chain links e after prev (nil for the first event) and seals it.
func (e *Event) Chain(prev *Event, at time.Time) {
	e.Seq, e.PrevHash = 1, GenesisHash
	if prev != nil {
		e.Seq, e.PrevHash = prev.Seq+1, prev.Hash
	}
	e.At = at.UTC().Truncate(time.Microsecond)
	e.Hash = e.ComputeHash()
}
