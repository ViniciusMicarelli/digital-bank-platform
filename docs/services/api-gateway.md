# api-gateway

## Responsibility

The single public entry point of the platform. It translates REST/JSON requests from clients into gRPC calls to internal services.

## Capabilities

- Routing `/v1/*` endpoints to backend services
- JWT validation (JWKS from `identity-service`) and scope checks
- Rate limiting (Redis, token bucket) per IP, user and route
- Idempotency key enforcement for state-changing requests
- Request validation against OpenAPI specs
- gRPC → HTTP error mapping (RFC 9457 Problem Details)
- CORS, security headers, request size limits
- Trace context creation and propagation

## Dependencies

| Type | Target |
|---|---|
| gRPC | All domain services |
| Redis | Rate limiting, idempotency cache |
| HTTP | `identity-service` JWKS endpoint |

## Data

Stateless. Uses only Redis for ephemeral data.

## Configuration

| Variable | Description |
|---|---|
| `HTTP_PORT` | Listen port (default `8080`) |
| `JWKS_URL` | Public keys endpoint for token validation |
| `REDIS_URL` | Redis connection string |
| `RATE_LIMIT_DEFAULT_RPS` | Default requests per second per user |
