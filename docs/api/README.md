# API Guidelines

## Overview

| Surface | Protocol | Contract location |
|---|---|---|
| Public API (clients) | REST / JSON over HTTPS | `contracts/openapi/` |
| Internal API (services) | gRPC / Protobuf over mTLS | `contracts/proto/` |

Only `api-gateway` is exposed to the internet.

## REST Conventions

### URLs and versioning

```text
https://api.<domain>/v1/accounts/{account_id}/balance
```

- Version in the path (`/v1`). A breaking change means a new version.
- Plural nouns for resources, kebab-case for multi-word paths (`/pix/keys`, `/scheduled-payments`).
- IDs are ULIDs (sortable, URL-safe).

### Main endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/users` | Sign up |
| `POST` | `/v1/auth/login` | Authenticate |
| `POST` | `/v1/auth/refresh` | Refresh tokens |
| `GET` | `/v1/me` | Current user profile |
| `GET` | `/v1/accounts/{id}` | Account details |
| `GET` | `/v1/accounts/{id}/balance` | Current balance |
| `GET` | `/v1/accounts/{id}/transactions` | Transaction feed |
| `GET` | `/v1/pix/keys` | List Pix keys |
| `POST` | `/v1/pix/keys` | Register a Pix key |
| `POST` | `/v1/pix/transfers` | Send a Pix transfer |
| `POST` | `/v1/pix/qr-codes` | Generate a Pix QR code |
| `POST` | `/v1/payments/boletos` | Pay a boleto |
| `GET` | `/v1/cards` | List cards |
| `POST` | `/v1/cards/virtual` | Create a virtual card |
| `PATCH` | `/v1/cards/{id}` | Block or unblock a card, change limits |
| `GET` | `/v1/credit/invoices` | List credit card invoices |
| `GET` | `/v1/statements/{year}/{month}` | Monthly statement |

The full specification lives in `contracts/openapi/`.

### Money representation

```json
{
  "amount": { "value": 5000, "currency": "BRL" }
}
```

`value` is an **integer in minor units** (cents). Floats are never used.

### Idempotency

State-changing requests (`POST`, `PATCH`, `DELETE`) **must** send:

```http
Idempotency-Key: 6c1b0b9e-6f0e-4b3a-9e3e-8a3b2f9c1d2a
```

### Pagination

Cursor-based:

```http
GET /v1/accounts/{id}/transactions?limit=20&cursor=eyJpZCI6...
```

```json
{
  "data": [ ... ],
  "pagination": { "next_cursor": "eyJpZCI6...", "has_more": true }
}
```

### Errors

Errors follow [RFC 9457 (Problem Details)](https://www.rfc-editor.org/rfc/rfc9457):

```json
{
  "type": "https://docs.<domain>/errors/insufficient-funds",
  "title": "Insufficient funds",
  "status": 422,
  "code": "INSUFFICIENT_FUNDS",
  "detail": "The account balance is lower than the requested amount.",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"
}
```

| HTTP | When |
|---|---|
| `400` | Malformed request / validation error |
| `401` | Missing or invalid token |
| `403` | Authenticated but not allowed |
| `404` | Resource not found (or not owned by the caller) |
| `409` | Conflict / idempotency key reused with a different payload |
| `422` | Business rule violation (insufficient funds, limit exceeded...) |
| `429` | Rate limited |
| `5xx` | Unexpected errors (never leak internals) |

### Standard headers

| Header | Direction | Purpose |
|---|---|---|
| `Authorization: Bearer <jwt>` | Request | Authentication |
| `Idempotency-Key` | Request | Safe retries |
| `X-Device-Id` | Request | Device binding |
| `traceparent` | Both | W3C trace context |
| `X-Request-Id` | Response | Correlation |

## gRPC Conventions

- Package naming: `bank.<service>.v1` (for example `bank.ledger.v1`).
- One `service` per bounded context. Methods are verbs (`PostEntry`, `GetBalance`).
- Use `google.protobuf.Timestamp` for time and a shared `bank.common.v1.Money` for money.
- Errors use standard gRPC status codes with `google.rpc.ErrorInfo` details.
- Linting and breaking-change detection with `buf lint` and `buf breaking`.

```protobuf
syntax = "proto3";

package bank.ledger.v1;

service LedgerService {
  rpc PostEntry(PostEntryRequest) returns (PostEntryResponse);
  rpc GetBalance(GetBalanceRequest) returns (GetBalanceResponse);
}
```
