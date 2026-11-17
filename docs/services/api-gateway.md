# api-gateway

## Responsibility

The only public entry point. It exposes a REST/JSON API and translates it to the internal gRPC services ([ADR-0006](../decisions/0006-grpc-internal-rest-external.md)). It holds no data and no business rules beyond the edge concerns below.

## Edge Concerns

| Concern | How |
|---|---|
| Authentication | `Authorization: Bearer <JWT>` (HS256, 15 min, issuer `digital-bank`, expiry required; `alg: none` and other algorithms are refused). The subject is the customer ID. |
| Authorization | Every `/v1/accounts/{id}/...` route loads the account and checks it belongs to the caller. Other customers' accounts answer **404**, so their existence is not revealed. |
| Idempotency | Money-moving `POST`s require an `Idempotency-Key` header. It is scoped to the customer (`<customer>:<key>`) and passed down to the sagas, so retries never move money twice. |
| Rate limiting | Token bucket per customer (20 req/s, burst 40). In-memory per replica; a shared limiter (Redis) is the next step for several replicas. |
| Validation | Unknown JSON fields are rejected, bodies are capped at 64 KiB, amounts must be decimal strings (`"10.50"`), never JSON numbers. |
| Errors | One format: `{"error": {"code", "message", "correlation_id"}}`. Codes come from the internal `ErrorInfo` reasons (`INSUFFICIENT_FUNDS`, `LIMIT_EXCEEDED`...). |
| Tracing | `X-Correlation-ID` is accepted when safe (`[A-Za-z0-9_.-]{1,64}`), otherwise generated, and returned on every response. OpenTelemetry spans are named after the route. |
| Metrics | `http_requests_total` and `http_request_duration_seconds` labelled by route pattern (never raw paths with IDs). |

## Development Switches

| Variable | Effect |
|---|---|
| `AUTH_DEV_LOGIN=true` | `POST /v1/auth/token` issues a token for any `customer_id`. Stand-in for identity-service; never enable it in production. |
| `SANDBOX=true` | `POST /v1/sandbox/pix-in` simulates a Pix from another bank, to fund demo accounts. |

## Endpoints

See the [REST API reference](../api/README.md).
