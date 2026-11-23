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
| 403 | Sensitive action without a fresh step-up token | `STEP_UP_REQUIRED` |
| 404 | Not found, or not yours | `NOT_FOUND` |
| 409 | Same key, different request; or still processing | `IDEMPOTENCY_CONFLICT`, `IN_PROGRESS` |
| 422 | Business refusal | `INSUFFICIENT_FUNDS`, `LIMIT_EXCEEDED`, `ACCOUNT_NOT_ACTIVE`, `RECEIVER_BANK_REJECTED`, `TOO_MANY_KEYS`, `QR_CODE_UNAVAILABLE`, `REFUND_TOO_LARGE`, `NOT_REFUNDABLE`, `CHARGE_NOT_ALLOWED`, `MANDATE_NOT_ACTIVE`, `CREDIT_LINE_EXISTS`, `PAYMENT_TOO_LARGE`, `INVOICE_NOT_PAYABLE`, `BOLETO_ALREADY_PAID`, `BOLETO_EXPIRED`, `DUPLICATE_PAYMENT`, `SETTLEMENT_REJECTED`, `SCHEDULE_AFTER_DUE_DATE`, `KYC_REQUIRED`, `DOCUMENT_MISMATCH`, `DOCUMENT_IN_USE`, `IDENTITY_LOCKED` |
| 429 | Rate limited | `RATE_LIMITED` |
| 503 | A dependency is down; retry with the same key | `RETRY_LATER` |

- **Pagination**: `?limit=` and an opaque `?cursor=` (`next_cursor` in the response).
- **CORS**: browsers on another origin (e.g. the mobile app's web build) must be listed in the gateway's `CORS_ALLOWED_ORIGINS`; the web app is same-origin and native apps do not need it.
- **Correlation**: send `X-Correlation-ID` to trace a request across services; it is echoed back.

## Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/auth/register` | Sign up `{email, password, full_name}` |
| `POST` | `/v1/auth/login` | `{email, password, totp_code?}` → access + refresh tokens |
| `POST` | `/v1/auth/refresh` | `{refresh_token}` → new pair (the old token stops working) |
| `POST` | `/v1/auth/logout` | `{refresh_token}` → ends the session |
| `POST` | `/v1/auth/step-up` | `{password, totp_code?}` → `step_up_token` (5 min), sent as `X-Step-Up-Token` on sensitive actions (🔐) |
| `POST` | `/v1/auth/mfa/totp` | Start TOTP enrollment → `{secret, otpauth_uri}` |
| `POST` | `/v1/auth/mfa/totp/confirm` | `{code}` → enables TOTP |
| `GET` | `/v1/me` | Who is signed in: `{customer_id, role, mfa}` (`CUSTOMER` or `ANALYST`; `mfa` when the session has a second factor) |
| `GET` | `/v1/me/sessions` | Where the customer is signed in: `[{id, device, ip, created_at, last_used_at, current}]`, most recently used first |
| `DELETE` | `/v1/me/sessions/{id}` | End one session → 204 (404 for someone else's or one already ended) |
| `POST` | `/v1/me/sessions/end-others` | End every session but the one asking → `{ended}` (409 `SESSION_UNKNOWN` for a token from before sessions existed) |
| `GET` · `PUT` | `/v1/me/profile` | Profile `{full_name, cpf, birth_date, phone, address{postal_code, street, number, complement, district, city, state}}` (CPF masked in answers) |
| `POST` | `/v1/me/kyc` | Start identity verification → `kyc_status` (`APPROVED`, `REJECTED`, `MANUAL_REVIEW`, `PENDING`) |
| `POST` | `/v1/sandbox/kyc/review` | *(sandbox)* `{approve, reason?}` decide a manual review |
| `POST` | `/v1/accounts` | Open an account (requires KYC; name and CPF come from the verified profile; `document`, if sent, must match) · Idempotency-Key |
| `GET` | `/v1/accounts` | My accounts |
| `GET` | `/v1/accounts/{id}` | Account details |
| `GET` | `/v1/accounts/{id}/balance` | `available`, `posted`, `pending` |
| `GET` | `/v1/accounts/{id}/tax-report` | Annual income report (`?year=YYYY`, last year by default; closed accounts too): holder (CPF masked), checking and each savings box's balance on 31/12 of the year before and of the year, yields in the year; `complete` is false while the year runs |
| `GET` | `/v1/accounts/{id}/closure` | Can the account be closed now: `{can_close, blockers [{code: BALANCE \| SAVINGS \| CREDIT_DEBT \| PAYMENTS_IN_PROGRESS, amount?, count?}], will_cancel {pix_schedules, boleto_schedules, pix_automatico, empty_savings_boxes}}` |
| `POST` | `/v1/accounts/{id}/close` | 🔐 Close the account at the holder's request `{reason?}`: 409 `CLOSURE_BLOCKED` (with the check) while something blocks it; otherwise scheduled Pix and boletos, Pix Automático authorizations and empty boxes are cancelled, the account is closed, and `account.closed` cancels cards and removes Pix keys. The statement and receipts stay readable |
| `GET` / `PUT` | `/v1/accounts/{id}/limits` | Limits and today's usage / change limits |
| `GET` | `/v1/accounts/{id}/statement` | Feed, newest first (`?limit=`, `?cursor=`); filters `?direction=credit\|debit` and `?q=` (words in the counterparty or description, any case, matched literally); the cursor works within a filter |
| `POST` | `/v1/accounts/{id}/transfers` | `{to_account_id, amount, description}` · Idempotency-Key |
| `GET` / `POST` | `/v1/accounts/{id}/pix/keys` | List / register `{type: CPF\|EMAIL\|PHONE\|EVP, value}` |
| `DELETE` | `/v1/accounts/{id}/pix/keys/{keyID}` | Delete a key |
| `GET` | `/v1/pix/keys/{key}` | Look up a key before paying |
| `GET` | `/v1/accounts/{id}/pix/contacts` | Favorites first, then recent receivers: `{key, name, bank_name, nickname, favorite, times_sent, last_sent_at}` |
| `PUT` · `DELETE` | `/v1/accounts/{id}/pix/contacts/favorites` | Save a favorite `{key, nickname?}` (`OWN_KEY`, `TOO_MANY_FAVORITES`) / remove one `?key=` |
| `GET` | `/v1/accounts/{id}/pix/transfers?direction=incoming\|outgoing` | Pix sent / received (use a received one's `id` to refund it) |
| `POST` | `/v1/accounts/{id}/pix/transfers` | `{key, amount, description}` · Idempotency-Key |
| `POST` | `/v1/accounts/{id}/pix/qr-codes` | Issue a BR Code `{key, amount?, description, one_time, dynamic, expires_in_seconds}` → `{txid, payload}` |
| `POST` / `GET` | `/v1/accounts/{id}/pix/schedules` | Schedule `{key, amount, start_date, frequency: ONCE\|WEEKLY\|MONTHLY, occurrences}` (Idempotency-Key) / list |
| `DELETE` | `/v1/accounts/{id}/pix/schedules/{scheduleID}` | Cancel a schedule |
| `POST` / `GET` | `/v1/accounts/{id}/pix/mandates` | Pix Automático: request `{receiver_key, payer_key, max_amount, frequency, description}` / list |
| `POST` | `/v1/accounts/{id}/pix/mandates/{mandateID}/approve` · `/reject` · `/revoke` | Payer decides; either side revokes |
| `POST` | `/v1/accounts/{id}/pix/mandates/{mandateID}/charges` | Receiver charges `{amount, due_date}` (Idempotency-Key) |
| `GET` | `/pix/qr/v2/{txid}` · `/pix/qr/jwks.json` | *(public)* Signed payload of a dynamic QR code, verification keys |
| `POST` | `/v1/sandbox/pix/run-schedules` | *(sandbox)* Run schedules due on `{as_of}` |
| `POST` | `/v1/pix/qr-codes/decode` | `{payload}` → receiver, amount, txid |
| `POST` | `/v1/accounts/{id}/pix/qr-payments` | Pay a BR Code `{payload, amount?}` · Idempotency-Key |
| `POST` | `/v1/accounts/{id}/pix/transfers/{transferID}/refunds` | Refund a received Pix `{amount, reason}` · Idempotency-Key |
| `POST` / `GET` | `/v1/accounts/{id}/cards` | Issue a debit card `{kind: PHYSICAL\|VIRTUAL\|TEMPORARY}` (Idempotency-Key; `TEMPORARY`: single use, 24 h, `TOO_MANY_TEMPORARY_CARDS`) / list cards (`valid_until` on temporary ones) |
| `POST` | `/v1/accounts/{id}/cards/{cardID}/block` · `/unblock` | Block (`{reason}`) / unblock |
| `PUT` | `/v1/accounts/{id}/cards/{cardID}/controls` | `{online, international, contactless}` |
| `PUT` · `DELETE` | `/v1/accounts/{id}/cards/{cardID}/monthly-limit` | Spending limit per month `{amount}` (whole reais) / remove it; cards then carry `monthly_limit {amount, used}` |
| `POST` | `/v1/accounts/{id}/cards/{cardID}/reveal` | 🔐 Card number, expiry, CVV |
| `POST` | `/v1/accounts/{id}/cards/{cardID}/tokens` | 🔐 `{wallet, device_name}` → wallet token (`token_pan` returned once) |
| `GET` | `/v1/accounts/{id}/cards/{cardID}/tokens` | Active and suspended tokens |
| `POST` | `/v1/accounts/{id}/cards/{cardID}/tokens/{tokenID}/suspend` | Suspend one device |
| `POST` | `/v1/accounts/{id}/cards/{cardID}/tokens/{tokenID}/resume` | 🔐 Resume it |
| `DELETE` | `/v1/accounts/{id}/cards/{cardID}/tokens/{tokenID}` | Delete it for good |
| `GET` | `/v1/accounts/{id}/cards/{cardID}/purchases` | The card's purchases, newest first, with `refunded_amount` and the latest `dispute` |
| `POST` | `/v1/accounts/{id}/cards/{cardID}/purchases/{authID}/disputes` | Dispute a cleared purchase `{reason, amount?, description?}` (`NOT_RECOGNIZED` blocks the card) → 201 (`NOT_CLEARED`, `DISPUTE_WINDOW_CLOSED`, `DISPUTE_ALREADY_OPEN`, `NOTHING_TO_DISPUTE`) |
| `GET` | `/v1/accounts/{id}/disputes` | The account's disputes, newest first |
| `GET` | `/v1/accounts/{id}/statement/{itemID}/receipt` | Receipt of a statement item: parties, accounts, Pix id, `authentication_code` |
| `GET` | `/v1/accounts/{id}/statement.csv` | CSV export `?from=YYYY-MM-DD&to=YYYY-MM-DD` (inclusive, current month by default, up to a year): `;` separated, decimal comma, UTF-8 BOM |
| `GET` | `/v1/receipts/{code}` | *(public, rate limited per IP)* Check a receipt's authentication code; names masked (`RECEIPT_NOT_FOUND`) |
| `GET` | `/v1/accounts/{id}/spending` | Spending of a month (`?month=YYYY-MM`, current by default): `total`, `previous_total`, `categories` (FOOD, TRANSPORT, SHOPPING, SUBSCRIPTIONS, HEALTH, LEISURE, TRAVEL, HOME, EDUCATION, BILLS, TRANSFERS, OTHER), top five `merchants` (eventually consistent) |
| `GET` | `/v1/accounts/{id}/budgets` | Monthly budgets `[{category, amount, spent}]` (`?month=YYYY-MM`), `TOTAL` first |
| `PUT` · `DELETE` | `/v1/accounts/{id}/budgets/{category}` | Set `{amount}` / remove the budget on a category or `total` → 204 (`INVALID_BUDGET`); alerts at 80% and 100% arrive as notifications |
| `GET` · `POST` | `/v1/accounts/{id}/savings` | Savings boxes with `total` and `yielded` / open one `{name, goal?}` (Idempotency-Key) → 201 (`TOO_MANY_BOXES`) |
| `GET` · `PUT` · `DELETE` | `/v1/accounts/{id}/savings/{boxID}` | A box with its `movements` / rename and set the goal `{name, goal?}` / close it, everything back to the account (Idempotency-Key) |
| `POST` | `/v1/accounts/{id}/savings/{boxID}/deposits` · `/withdrawals` | `{amount}` (Idempotency-Key); 422 `INSUFFICIENT_FUNDS` with the movement when there is not enough |
| `PUT` | `/v1/accounts/{id}/savings/{boxID}/round-up` | `{enabled}`: round debit card purchases up into this box (one per account); boxes carry `round_up`, movements `source` (ROUND_UP, SCHEDULE) |
| `PUT` · `DELETE` | `/v1/accounts/{id}/savings/{boxID}/schedule` | Monthly deposit `{amount, day}` (day 1–28; the box then has `schedule {amount, day, next_run, last_failure}`) / remove it |
| `POST` | `/v1/sandbox/accounts/{id}/savings/{boxID}/yield` | *(sandbox)* Pay `{days}` of yield at once |
| `POST` | `/v1/sandbox/accounts/{id}/savings/{boxID}/schedule/run` | *(sandbox)* Make the next monthly deposit now; 422 `INSUFFICIENT_FUNDS` / `NO_SCHEDULE` |
| `POST` | `/v1/sandbox/card-network/authorizations` | *(sandbox)* Network authorization `{pan, expiry_month, expiry_year, cvv, amount, merchant_name, mcc, country, channel, function?}` (`function`: `DEBIT` default, or `CREDIT`; `installments`: 2 to 12 on credit) |
| `POST` | `/v1/sandbox/card-network/authorizations/{id}/clear` · `/reverse` | *(sandbox)* Clearing `{amount?}` / merchant reversal |
| `POST` | `/v1/sandbox/card-network/authorizations/{id}/refund` | *(sandbox)* Merchant refund `{amount, refund_reference?}` of a cleared purchase (`REFUND_TOO_LARGE`) |
| `GET` | `/v1/notifications` | Inbox `?unread=true&before=<id>&limit=`, with `unread_count` |
| `POST` | `/v1/notifications/read` | `{ids?}` mark read (all unread when empty) |
| `GET` · `PUT` | `/v1/notifications/preferences` | `{push, email}` |
| `GET` | `/v1/notifications/stream` | Live notifications as Server-Sent Events (`id`, `event: notification`, JSON `data`); send `Last-Event-ID` to resume |
| `POST` | `/v1/boletos/lookup` | `{line}` → payee, due date, amount with charges if overdue |
| `POST` | `/v1/accounts/{id}/boleto-payments` | Pay `{line, amount?, scheduled_for?}` (Idempotency-Key); refusals are 422 with the reason |
| `GET` | `/v1/accounts/{id}/boleto-payments` | Payments, newest first |
| `GET` · `DELETE` | `/v1/accounts/{id}/boleto-payments/{paymentID}` | Read / cancel a scheduled payment |
| `POST` | `/v1/sandbox/boletos` | *(sandbox)* Issue a valid boleto `{amount, due_date, payee_name?, behavior?: PAID \| REJECT}` |
| `POST` | `/v1/accounts/{id}/credit` | Open the credit line `{closing_day?}` (Idempotency-Key) |
| `GET` | `/v1/accounts/{id}/credit` | Limit, used, available |
| `GET` | `/v1/accounts/{id}/credit/invoices` | Invoices, newest first |
| `GET` · `POST` | `/v1/accounts/{id}/credit/invoices/{invoiceID}/plans` | Split a closed invoice: the options `{installments, installment, total, interest, monthly_rate, annual_cost}` / choose one `{installments}` (Idempotency-Key) → 201 (`INVOICE_NOT_PLANNABLE`, `INVOICE_TOO_SMALL`, `PAYMENT_IN_PROGRESS`). Invoices also carry `minimum_payment` |
| `POST` | `/v1/accounts/{id}/credit/limit` | Ask for a new limit `{amount}` (Idempotency-Key) → 200 `APPROVED` / `REJECTED` (`HAS_OVERDUE_INVOICE`, `LOW_SCORE`), 202 `IN_REVIEW` (`LIMIT_BELOW_USED`, `LIMIT_REQUEST_IN_REVIEW`); each request carries the `score` it was analyzed with |
| `GET` | `/v1/accounts/{id}/credit/score` | Behaviour score `{score (0–1000), band, factors [{code, points, value}], auto_increase_bp}` |
| `GET` | `/v1/accounts/{id}/credit/limit-requests` | The account's limit requests, newest first |
| `GET` | `/v1/accounts/{id}/credit/installments` | Installment purchases with installments to come: `{description, total, installments, billed, installment_amount, remaining}` (the line also has `upcoming_installments`) |
| `GET` | `/v1/accounts/{id}/credit/invoices/{invoiceID}` | Invoice with its items |
| `POST` | `/v1/accounts/{id}/credit/invoices/{invoiceID}/payments` | Pay `{amount}` from the account (Idempotency-Key) |
| `POST` | `/v1/sandbox/accounts/{id}/credit/cycle` | *(sandbox)* `{action: CLOSE \| EXPIRE}` close the invoice now / make it overdue |
| `POST` | `/v1/sandbox/pix-in` | *(sandbox only)* Simulate an incoming Pix `{key, amount, payer_name}` |
| `GET` | `/v1/backoffice/kyc/reviews` | 👤 Verifications in manual review, oldest first |
| `POST` | `/v1/backoffice/kyc/reviews/{customerID}` | 👤 Decide `{approve, reason}` (reason required to reject); the analyst is recorded |
| `GET` | `/v1/backoffice/customers/{customerID}` | 👤 Profile, verification and accounts with balances |
| `POST` | `/v1/backoffice/accounts/{id}/block` | 👤 Block `{reason}` |
| `POST` | `/v1/backoffice/accounts/{id}/unblock` | 👤 Ask to unblock `{reason}` → 202 with a four-eyes approval (`ACCOUNT_NOT_BLOCKED`, `APPROVAL_ALREADY_PENDING`) |
| `GET` | `/v1/backoffice/approvals` | 👤 Four-eyes requests (`?status=PENDING` by default, or `APPROVED`, `EXECUTED`, `REJECTED`, `FAILED`, `EXPIRED`, `ALL`), with the account and its customer |
| `POST` | `/v1/backoffice/approvals/{id}/approve` · `/reject` | 👤 The second analyst's decision `{reason}` (required to reject); approving runs the action → `EXECUTED` or `FAILED` (`SAME_ANALYST`, `APPROVAL_NOT_PENDING`, `APPROVAL_EXPIRED`) |
| `GET` | `/v1/backoffice/disputes` | 👤 Card disputes (`?status=OPEN` by default, or `WON`, `LOST`, `ALL`), with the account holder and customer |
| `POST` | `/v1/backoffice/disputes/{id}/decide` | 👤 `{won, reason}` (reason required to reject); won refunds the amount; audited on the account |
| `GET` | `/v1/backoffice/limit-requests` | 👤 Limit requests (`?status=IN_REVIEW` by default, or `APPROVED`, `REJECTED`, `ALL`) with the holder and what is used |
| `POST` | `/v1/backoffice/accounts/{id}/credits` | 👤 Ask for a manual credit `{amount, reason}` (up to R$ 50.000,00; the reason is shown to the customer) → 202 with a `MANUAL_CREDIT` approval; another analyst's approval posts it; audited |
| `POST` | `/v1/backoffice/limit-requests/{id}/decide` | 👤 `{approve, granted?, reason}` (reason required to reject; may grant less); audited on the account. Approving more than R$ 10.000,00 (reason required) answers 202 `AWAITING_SECOND_ANALYST` with a `LIMIT_INCREASE` approval for another analyst |
| `GET` | `/v1/backoffice/audit` | 👤 Audit trail, newest first (`?target_id=`, `?actor_id=`, `?limit=`) |
| `GET` | `/v1/backoffice/audit/verify` | 👤 Recompute the hash chain → `{intact, events, broken_at}` |
| `GET` | `/healthz`, `/metrics` | Health and Prometheus metrics |

👤 Back office: the access token's `role` must be `ANALYST` (403 `STAFF_ONLY`) and the session must have a second factor (403 `MFA_REQUIRED`). Actions are written to the audit trail first and refused when it is unavailable (503 `AUDIT_UNAVAILABLE`).

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
