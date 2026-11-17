# API Documentation

This directory contains API documentation for the Digital Bank Platform.

## Overview

The platform exposes two types of APIs:

1. **External API (REST)**: JSON-based REST API exposed through the API Gateway for client applications (web and mobile).
2. **Internal API (gRPC)**: Protocol Buffers-based gRPC API for inter-service communication.

## External API

| Specification | Location |
|--------------|----------|
| OpenAPI 3.1 | [contracts/openapi/spec.yaml](../../contracts/openapi/spec.yaml) |

Generated clients:
- TypeScript: `npm run generate:openapi`
- Kotlin: `make generate-kotlin`
- Swift: `make generate-swift`

### Authentication

All external API requests require a JWT access token in the `Authorization: Bearer <token>` header. Tokens are obtained through the identity service's login endpoint.

See [https://swagger.io/docs/open-source-tools/swagger-ui/](https://swagger.io/docs/open-source-tools/swagger-ui/) for interactive API documentation.

### Rate Limiting

External API endpoints are protected by rate limiting:
- Public endpoints: 100 requests per minute per IP
- Authenticated endpoints: 1000 requests per minute per user

Rate limit headers are included in all responses:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 99
Retry-After: 60
```

## Internal API

| Protocol | Location |
|----------|----------|
| Protocol Buffers | [contracts/proto/](../../contracts/proto/) |

Services expose specific gRPC services defined in Protocol Buffer files. Example:

```protobuf
// services/identity-service/internal/ports/identity.proto
syntax = "proto3";

service IdentityService {
  rpc Login (LoginRequest) returns (LoginResponse);
  rpc RefreshToken (RefreshRequest) returns (RefreshResponse);
  rpc VerifyMFA (MFARequest) returns (MFAResponse);
}
```

### Authentication

Services authenticate via mTLS using the service mesh. Each service presents a TLS certificate during gRPC connection establishment.

### Error Handling

gRPC errors follow the [gRPC status codes](https://grpc.io/docs/guides/status/):
- `UNKNOWN`: General unknown error
- `INVALID_ARGUMENT`: Client sent invalid arguments
- `NOT_FOUND`: Resource not found
- `UNAUTHENTICATED`: Missing or invalid credentials
- `UNAVAILABLE`: Service unavailable

Error codes are mapped from domain errors via `go-errors`. See [packages/go-errors/README.md](../../packages/go-errors/README.md).

## API Versioning

### External API

Versioning via URL path with semantic versioning:
```
/v1/users
/v2/users
```

Breaking changes:
- Bump minor version
- Document deprecation in OpenAPI

New versions are:
- Auto-routed by Accept-Version header
- Default to latest stable version

### Internal API

Versioning via separate services:
```
identity-service-v1
identity-service-v2
```

## API Guidelines

1. **External API** follows [REST best practices](https://www.infoq.com/articles/restful-service-arch/):
   - Resources have nouns, actions have verbs
   - HTTP methods have consistent semantics (GET for read, POST for create, PUT for update, DELETE for remove)
   - Response status codes have standard meanings

2. **Internal API** follows gRPC design patterns:
   - Each service is a single logical unit of work
   - Methods are verbs matching the business operation
   - Request/response objects are well-defined
   - Errors follow domain error mappings

## Generated Code

- Generated clients in `packages/`
- Generated SDKs per target language in `contracts/`
- Regenerate: `make generate`

## API Testing

API tests are defined in service test directories. Example:

```go
// services/identity-service/internal/application/handlers_test.go
func TestLogin(t *testing.T) {
  // Test case
}
```

Integration tests:
```bash
make test-api
```

## API Development Workflow

1. Define contracts in `contracts/` directory
2. Generate code: `make generate`
3. Implement handlers in service
4. Run tests: `make test`
5. Update API specs if breaking changes: `make generate-openapi`

## Directory Structure

| File | Description |
|------|-------------|
| [README.md](README.md) | This API documentation |
| [external.md](./external.md) | External REST API documentation |
| [internal.md](./internal.md) | Internal gRPC API documentation |
| [authentication.md](./authentication.md) | Authentication and authorization |
| [error-codes.md](./error-codes.md) | API error code definitions |
| [versioning.md](./versioning.md) | API versioning strategy |
| [guidelines.md](./guidelines.md) | API development guidelines |
