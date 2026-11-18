# backoffice-service

## Responsibility

Makes what bank staff do **accountable**: a tamper-evident audit trail of every back-office action and **four-eyes approvals**, where a sensitive action requested by one analyst only runs once a different analyst approves it. Only the api-gateway calls it; it calls account-service to carry out approved actions.

## Capabilities

| Capability | Status |
|---|---|
| Audit trail: append-only table (triggers refuse `UPDATE`, `DELETE`, `TRUNCATE`) | ✅ |
| Hash chain: each event stores the SHA-256 of the previous hash and its own fields (length-prefixed, details sorted, UTC microseconds); appends are serialized with an advisory lock, so the chain never forks | ✅ |
| `VerifyAudit` recomputes the chain and reports the first event that does not match (edited, removed or reordered) | ✅ |
| `ListAudit` by target (customer, account) and/or actor, newest first | ✅ |
| Four-eyes approvals for unblocking an account: one open request per account, 24 h to decide, the requester can never decide (`SAME_ANALYST`, also a database `CHECK`), a rejection needs a reason | ✅ |
| Approved actions run right away; a transient failure leaves the approval `APPROVED` and a recovery loop retries it (the unblock is idempotent: an account already active counts as done); a refusal settles it `FAILED` with the reason | ✅ |
| Every approval step (requested, approved / rejected, executed / failed, expired) is written to the trail in the same transaction | ✅ |
| More four-eyes actions (limit increases, manual credits, closing accounts), audit export to WORM storage | planned |

## Approval States

```mermaid
stateDiagram-v2
    [*] --> PENDING: analyst A requests
    PENDING --> APPROVED: analyst B approves
    PENDING --> REJECTED: analyst B rejects (reason)
    PENDING --> EXPIRED: 24 h without a decision
    APPROVED --> EXECUTED: action done
    APPROVED --> FAILED: action refused (e.g. ACCOUNT_NOT_BLOCKED)
    APPROVED --> APPROVED: outage, retried by the recovery loop
```

## Audit events

The gateway writes an event **before** each staff action and refuses the action when it cannot (fail closed, 503 `AUDIT_UNAVAILABLE`); when the action itself then fails, it adds `<ACTION>_FAILED` with the error.

| Action | Target | Written by |
|---|---|---|
| `CUSTOMER_FILE_VIEW` | customer | gateway (every look at personal data, LGPD) |
| `KYC_APPROVE`, `KYC_REJECT` | customer | gateway |
| `ACCOUNT_BLOCK` | account | gateway |
| `APPROVAL_REQUESTED`, `APPROVAL_APPROVED`, `APPROVAL_REJECTED`, `APPROVAL_EXECUTED`, `APPROVAL_FAILED`, `APPROVAL_EXPIRED` | account | backoffice-service |

## Configuration

| Variable | Default |
|---|---|
| `GRPC_ADDR` | `:9013` |
| `METRICS_ADDR` | `:9113` |
| `DATABASE_URL` | `postgres://bank:bank@localhost:54329/backoffice` |
| `ACCOUNT_ADDR` | `127.0.0.1:9002` |
| `RECOVERY_INTERVAL` | `30s` |
