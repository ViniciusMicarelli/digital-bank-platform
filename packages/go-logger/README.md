# go-logger

Structured logging using Go's standard library `slog`.

## Packages

### slog

Core logger implementation with contextual fields.

```
slog/
├── logger.go
├── fields.go
└── handler.go
```

### zap

Legacy Zap logger adapter (if needed).

```
zap/
└── logger.go
```

## Usage

```go
import "digital-bank/packages/go-logger"
```
