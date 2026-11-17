# go-security

Security primitives for authentication, authorization, and cryptography.

## Packages

### auth

Authentication and token management (JWT).

```
auth/
├── jwt.go
├── token.go
└── verifier.go
```

### mtls

mTLS certificate management and validation.

```
mtls/
├── cert.go
├── verifier.go
└── tls.go
```

### crypto

Encryption and hashing utilities.

```
crypto/
├── encrypt.go
├── hash.go
└── password.go
```

### authn

Authentication implementations (password, OAuth).

```
authn/
├── password.go
└── oauth.go
```

## Usage

```go
import "digital-bank/packages/go-security"
```
