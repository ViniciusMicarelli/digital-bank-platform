package domain

import (
	"testing"
	"time"
)

func TestChain(t *testing.T) {
	at := time.Date(2026, 11, 18, 10, 0, 0, 123456789, time.FixedZone("BRT", -3*3600))
	first := Event{ActorID: "a1", Action: "KYC_APPROVED", TargetType: "customer", TargetID: "c1", Details: map[string]string{"b": "2", "a": "1"}}
	first.Chain(nil, at)
	if first.Seq != 1 || first.PrevHash != GenesisHash || first.At.Nanosecond() != 123456000 {
		t.Fatalf("first = %+v", first)
	}
	second := Event{ActorID: "a1", Action: "ACCOUNT_BLOCKED"}
	second.Chain(&first, at)
	if second.Seq != 2 || second.PrevHash != first.Hash || second.Hash == first.Hash {
		t.Fatalf("second = %+v", second)
	}
	// Any change breaks the hash; the encoding has no ambiguity.
	changed := first
	changed.Details = map[string]string{"a": "1", "b": "3"}
	if changed.ComputeHash() == first.Hash {
		t.Fatal("details are covered")
	}
	shifted := first
	shifted.ActorID, shifted.Action = "a1K", "YC_APPROVED"
	if shifted.ComputeHash() == first.Hash {
		t.Fatal("fields are length-prefixed")
	}
	local := first
	local.At = first.At.In(time.FixedZone("BRT", -3*3600))
	if local.ComputeHash() != first.Hash {
		t.Fatal("the hash does not depend on the time zone")
	}
}

func TestDecide(t *testing.T) {
	now := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	if _, err := NewApproval(KindUnblockAccount, "acc", " ", "a1", nil, now); err == nil {
		t.Fatal("a reason is required")
	}
	a, err := NewApproval(KindUnblockAccount, "acc", "cliente confirmou a identidade", "a1", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := AsRefused(a.Decide("a1", true, "", now)); r == nil || r.Reason != "SAME_ANALYST" {
		t.Fatalf("the requester cannot approve: %v", r)
	}
	if err := a.Decide("a2", false, "", now); err == nil {
		t.Fatal("a rejection needs a reason")
	}
	late := a
	if r, _ := AsRefused(late.Decide("a2", true, "", now.Add(ApprovalTTL))); r == nil || r.Reason != "APPROVAL_EXPIRED" {
		t.Fatalf("expired: %v", r)
	}
	if err := a.Decide("a2", true, "", now); err != nil || a.Status != StatusApproved || a.DecidedBy != "a2" {
		t.Fatalf("approve = %+v %v", a, err)
	}
	if r, _ := AsRefused(a.Decide("a3", true, "", now)); r == nil || r.Reason != "APPROVAL_NOT_PENDING" {
		t.Fatalf("decided twice: %v", r)
	}
	a.Settle("ACCOUNT_NOT_BLOCKED")
	if a.Status != StatusFailed || a.FailureReason != "ACCOUNT_NOT_BLOCKED" {
		t.Fatalf("settle = %+v", a)
	}
}

func TestLimitIncreaseApproval(t *testing.T) {
	now := time.Now()
	if _, err := NewApproval(KindLimitIncrease, "req-1", "renda comprovada", "a1", map[string]string{"account_id": "acc"}, now); err == nil {
		t.Fatal("a limit increase needs the amount")
	}
	a, err := NewApproval(KindLimitIncrease, "req-1", "renda comprovada", "a1", map[string]string{"account_id": "acc", "granted_minor": "1500000"}, now)
	if err != nil || a.Params["granted_minor"] != "1500000" {
		t.Fatalf("approval = %+v %v", a, err)
	}
}
