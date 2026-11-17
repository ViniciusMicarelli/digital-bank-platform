# API

## Two APIs

| | Public (edge) | Internal |
|---|---|---|
| Protocol | REST + JSON over HTTP | gRPC + Protocol Buffers |
| Served by | `api-gateway` (`:8000` locally) | each service (`:9001`–`:9005`) |
| Contract | this page | [`contracts/proto`](../../contracts/proto) |
| Auth | Bearer JWT per customer | service-to-service (mTLS planned) |

## Conventions (public API)

- **Money** is `{"amount": "10.50", "currency": "BRL"}` in responses and a decimal string (`"10.50"`) in requests. Never a JSON number.
- **Idempotency**: `POST` endpoints that move money or create accounts require `Idempotency-Key` (8–100 characters). Repeating a request with the same key returns the original result.
- **Errors**: `{"error": {"code": "INSUFFICIENT_FUNDS", "message": "...", "correlation_id": "..."}}`.

| HTTP | When | Example codes |
|---|---|---|
| 400 | Malformed request | `INVALID_JSON`, `INVALID_AMOUNT`, `IDEMPOTENCY_KEY_REQUIRED` |
| 401 | Missing or invalid token / credentials | `UNAUTHENTICATED`, `INVALID_CREDENTIALS`, `MFA_REQUIRED`, `ACCOUNT_LOCKED`, `REFRESH_TOKEN_REUSED` |
| 404 | Not found, or not yours | `NOT_FOUND` |
| 409 | Same key, different request; or still processing | `IDEMPOTENCY_CONFLICT`, `IN_PROGRESS` |
| 422 | Business refusal | `INSUFFICIENT_FUNDS`, `LIMIT_EXCEEDED`, `ACCOUNT_NOT_ACTIVE`, `RECEIVER_BANK_REJECTED`, `TOO_MANY_KEYS`, `QR_CODE_UNAVAILABLE`, `REFUND_TOO_LARGE`, `NOT_REFUNDABLE` |
| 429 | Rate limited | `RATE_LIMITED` |
| 503 | A dependency is down; retry with the same key | `RETRY_LATER` |

- **Pagination**: `?limit=` and an opaque `?cursor=` (`next_cursor` in the response).
- **Correlation**: send `X-Correlation-ID` to trace a request across services; it is echoed back.

## Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/auth/register` | Sign up `{email, password, full_name}` |
| `POST` | `/v1/auth/login` | `{email, password, totp_code?}` → access + refresh tokens |
| `POST` | `/v1/auth/refresh` | `{refresh_token}` → new pair (the old token stops working) |
| `POST` | `/v1/auth/logout` | `{refresh_token}` → ends the session |
| `POST` | `/v1/auth/mfa/totp` | Start TOTP enrollment → `{secret, otpauth_uri}` |
| `POST` | `/v1/auth/mfa/totp/confirm` | `{code}` → enables TOTP |
| `POST` | `/v1/accounts` | Open an account `{holder_name, document}` · Idempotency-Key |
| `GET` | `/v1/accounts` | My accounts |
| `GET` | `/v1/accounts/{id}` | Account details |
| `GET` | `/v1/accounts/{id}/balance` | `available`, `posted`, `pending` |
| `GET` / `PUT` | `/v1/accounts/{id}/limits` | Limits and today's usage / change limits |
| `GET` | `/v1/accounts/{id}/statement` | Feed, newest first |
| `POST` | `/v1/accounts/{id}/transfers` | `{to_account_id, amount, description}` · Idempotency-Key |
| `GET` / `POST` | `/v1/accounts/{id}/pix/keys` | List / register `{type: CPF\|EMAIL\|PHONE\|EVP, value}` |
| `DELETE` | `/v1/accounts/{id}/pix/keys/{keyID}` | Delete a key |
| `GET` | `/v1/pix/keys/{key}` | Look up a key before paying |
| `GET` | `/v1/accounts/{id}/pix/transfers?direction=incoming\|outgoing` | Pix sent / received (use a received one's `id` to refund it) |
| `POST` | `/v1/accounts/{id}/pix/transfers` | `{key, amount, description}` · Idempotency-Key |
| `POST` | `/v1/accounts/{id}/pix/qr-codes` | Issue a BR Code `{key, amount?, description, one_time, expires_in_seconds}` → `{txid, payload}` |
| `POST` | `/v1/pix/qr-codes/decode` | `{payload}` → receiver, amount, txid |
| `POST` | `/v1/accounts/{id}/pix/qr-payments` | Pay a BR Code `{payload, amount?}` · Idempotency-Key |
| `POST` | `/v1/accounts/{id}/pix/transfers/{transferID}/refunds` | Refund a received Pix `{amount, reason}` · Idempotency-Key |
| `POST` | `/v1/sandbox/pix-in` | *(sandbox only)* Simulate an incoming Pix `{key, amount, payer_name}` |
| `GET` | `/healthz`, `/metrics` | Health and Prometheus metrics |

`scripts/demo.sh` (`make demo`) walks through all of them.

## Example

```bash
curl -s -X POST localhost:8000/v1/auth/register -d '{"email":"ana@example.com","password":"correct horse battery staple","full_name":"Ana Souza"}'
TOKEN=$(curl -s -X POST localhost:8000/v1/auth/login -d '{"email":"ana@example.com","password":"correct horse battery staple"}' | jq -r .access_token)

curl -s -X POST localhost:8000/v1/accounts/$ACCOUNT/transfers \
  -H "Authorization: Bearer $TOKEN" -H "Idempotency-Key: 6f0c7a52-rent-nov" \
  -d '{"to_account_id":"…","amount":"120.00","description":"Aluguel"}'
```

```json
{
  "id": "01a1ea6f-fa49-7c54-a6ff-7b69cb59b49f",
  "status": "COMPLETED",
  "amount": { "amount": "120.00", "currency": "BRL" },
  "from_account_id": "…",
  "to_account_id": "…",
  "description": "Aluguel",
  "created_at": "2026-11-17T16:56:09.512Z"
}
```

## Internal gRPC Conventions

- Errors use standard gRPC codes plus a `google.rpc.ErrorInfo` detail whose `reason` callers branch on (`packages/go-platform/grpcserver.Error` / `Reason`).
- `FAILED_PRECONDITION`, `NOT_FOUND` and `INVALID_ARGUMENT` are business refusals; anything else is treated as technical and retried.
- `x-correlation-id` metadata and W3C `traceparent` are propagated on every call.
- Contracts are linted with `buf`; generated code is committed and CI checks it is up to date (`make generate`).
