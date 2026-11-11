# ledger-service

## Responsibility

The **source of truth for all money**. It records every movement as an immutable, balanced, double-entry journal entry.

See [ADR-0005](../decisions/0005-double-entry-ledger.md) and the [ledger data model](../database/README.md#ledger-model).

## Concepts

| Concept | Description |
|---|---|
| Ledger account | A bucket of value: a customer checking account, a Pix settlement account, a card clearing account, a revenue account... |
| Journal entry | An atomic movement containing two or more postings |
| Posting | A signed amount applied to a ledger account |
| Balance | `available` (usable) and `pending` (held) amounts per ledger account |

## Rules

- The sum of postings in every entry **must equal zero**.
- All postings in an entry use the same currency.
- Entries are append-only. Mistakes are fixed with **reversal entries**.
- Every entry has an `idempotency_key`: posting the same key twice returns the original entry.
- Balance updates use row-level locks (`SELECT ... FOR UPDATE`) in the same transaction as the postings, with a fixed lock ordering to avoid deadlocks.
- Customer accounts cannot go negative unless an overdraft limit is configured.

## Holds

Two-phase movements (card authorizations, outgoing Pix) use **holds**:

1. `Hold`: moves value from `available` to `pending`.
2. `Capture`: posts the final entry.
3. `Release`: returns the value to `available` (compensation).

## gRPC API

| Method | Description |
|---|---|
| `CreateLedgerAccount` | Create a ledger account |
| `PostEntry` | Post a balanced journal entry |
| `Hold` / `Capture` / `Release` | Two-phase movements |
| `ReverseEntry` | Post a reversal for an entry |
| `GetBalance` | Current balance |
| `ListPostings` | Postings for an account (paginated) |
| `GetBalanceAt` | Historical balance at a point in time |

## Events

| Publishes | Consumes |
|---|---|
| `ledger.entry.posted` | — (commands come via gRPC) |
| `ledger.hold.created` | |
| `ledger.hold.released` | |

## Data

`ledger_accounts`, `journal_entries`, `postings`, `balances`, `holds`, `outbox`.

## Testing

- Property-based tests: for any random sequence of valid operations, the global sum is zero and no customer balance goes negative.
- Concurrency tests: N parallel debits against the same account never overdraw it.
- A periodic reconciliation job checks the invariants in production.
