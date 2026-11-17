# go-observability

OpenTelemetry instrumentation for tracing and metrics.

## Packages

### trace

OpenTelemetry tracing setup and spans.

```
trace/
├── tracer.go
├── span.go
└── propagators.go
```

### metrics

OpenTelemetry metrics and instrument providers.

```
metrics/
├── metrics.go
├── provider.go
└── histogram.go
```

### propagation

Distributed context propagation (trace headers).

```
propagation/
├── jaeger.go
└── b3.go
```

## Usage

```go
import "digital-bank/packages/go-observability"
```
