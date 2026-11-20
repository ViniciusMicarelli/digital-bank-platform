# api-gateway

## Responsibility

The only public entry point. It exposes a REST/JSON API and translates it to the internal gRPC services ([ADR-0006](../decisions/0006-grpc-internal-rest-external.md)). It holds no data and no business rules beyond the edge concerns below.

## Edge Concerns

| Concern | How |
|---|---|
| Authentication | `Authorization: Bearer <JWT>` issued by [identity-service](identity-service.md): EdDSA only, issuer, audience and expiry required. Public keys come from the JWKS (cached 5 min, refetched on an unknown `kid` at most every 10 s). The gateway holds no signing secret. |
| Authorization | Every `/v1/accounts/{id}/...` route loads the account and checks it belongs to the caller. Other customers' accounts answer **404**, so their existence is not revealed. |
| Idempotency | Money-moving `POST`s require an `Idempotency-Key` header. It is scoped to the customer (`<customer>:<key>`) and passed down to the sagas, so retries never move money twice. |
| Client address | The peer address, unless it is a trusted proxy (`TRUSTED_PROXIES`, CIDRs, e.g. the web app's nginx or a load balancer): then `X-Forwarded-For` is read from the right and the first untrusted hop is the client, so forged entries on the left are ignored. Used by per-IP rate limits and recorded on sessions |
| Rate limiting | Token bucket per customer (20 req/s, burst 40), and per client IP on `/v1/auth/*` (2 req/s, burst 10) against credential stuffing. GCRA in Redis (`REDIS_ADDR`), so all replicas share one budget; Redis' clock is used, and if Redis is down requests pass (fail open, `ratelimit_redis_errors_total`). Without Redis, an in-memory bucket per replica. |
| Validation | Unknown JSON fields are rejected, bodies are capped at 64 KiB, amounts must be decimal strings (`"10.50"`), never JSON numbers. |
| Errors | One format: `{"error": {"code", "message", "correlation_id"}}`. Codes come from the internal `ErrorInfo` reasons (`INSUFFICIENT_FUNDS`, `LIMIT_EXCEEDED`...). |
| Tracing | `X-Correlation-ID` is accepted when safe (`[A-Za-z0-9_.-]{1,64}`), otherwise generated, and returned on every response. OpenTelemetry spans are named after the route. |
| Metrics | `http_requests_total` and `http_request_duration_seconds` labelled by route pattern (never raw paths with IDs). |

## Development Switches

| Variable | Effect |
|---|---|
| `SANDBOX=true` | `POST /v1/sandbox/pix-in` simulates a Pix from another bank, to fund demo accounts. |

## Endpoints

See the [REST API reference](../api/README.md).
