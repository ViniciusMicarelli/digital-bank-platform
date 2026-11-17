# go-common

Core shared utilities for the digital bank platform.

## Packages

### money

Currency calculations and money value representations.

```
money/
├── money.go
```

### ids

Identifier generation and validation utilities.

```
ids/
├── uuid.go
└── shortid.go
```

### pagination

Pagination helpers for API responses.

```
pagination/
├── page.go
└── query.go
```

### time

Time formatting and duration utilities.

```
time/
├── duration.go
└── format.go
```

## Usage

Import specific subpackages as needed:

```go
import "digital-bank/packages/go-common/money"
```
