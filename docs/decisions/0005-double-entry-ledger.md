# ADR-0005: Double-Entry Ledger as Source of Truth

- **Status:** Accepted
- **Date:** 2026-11-11

## Context

Storing balances as a mutable column (`UPDATE accounts SET balance = balance - 50`) loses history, makes auditing hard and is error-prone under concurrency.

## Decision

All money movements are recorded in `ledger-service` as **immutable journal entries** made of balanced postings (the sum is always zero). Balances are derived from postings and cached in a snapshot updated atomically.

## Alternatives Considered

- **Mutable balance column**: simple, but not auditable.
- **Pure event sourcing for every aggregate**: powerful, but excessive complexity for non-financial domains.

## Consequences

- ✅ Full audit trail, and any historical balance can be reconstructed.
- ✅ Errors are corrected with reversal entries instead of destructive updates.
- ✅ The invariant (sum = 0) can be checked continuously.
- ⚠️ The ledger becomes a critical, high-throughput component that must be heavily tested.
