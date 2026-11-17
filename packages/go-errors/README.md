# go-errors

Domain-specific error types with gRPC/HTTP status mapping.

## Packages

### domain

Domain-level errors with semantic meaning.

```
domain/
├── errors.go
```

### http

HTTP status code definitions.

```
http/
└── codes.go
```

### grpc

gRPC status code definitions.

```
grpc/
└── codes.go
```

### errors

Error wrapping and transformation utilities.

```
errors/
├── wrap.go
└── transform.go
```

## Usage

```go
import "digital-bank/packages/go-errors"
```
