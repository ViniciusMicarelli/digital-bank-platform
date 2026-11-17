# go-messaging

Kafka messaging infrastructure for event sourcing.

## Packages

### kafka

Kafka producer and consumer implementations.

```
kafka/
├── producer.go
└── consumer.go
```

### outbox

Transaction log outbox pattern for reliable event publishing.

```
outbox/
├── outbox.go
├── publisher.go
└── worker.go
```

### inbox

Event inbox pattern for reliable event consumption.

```
inbox/
├── inbox.go
├── consumer.go
└── worker.go
```

## Usage

```go
import "digital-bank/packages/go-messaging"
```
