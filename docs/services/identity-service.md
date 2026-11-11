# identity-service

## Responsibility

Authentication and session management: who the user is and whether this device and session can act on their behalf.

## Capabilities

- Credential management (password hashed with Argon2id, password reset)
- Login with risk-based MFA (TOTP / OTP via SMS or email)
- Token issuing: JWT access tokens (15 min) and rotating opaque refresh tokens
- Device registration and trust
- Session listing and revocation ("log out of all devices")
- Step-up authentication for sensitive operations
- JWKS endpoint with key rotation

## gRPC API

| Method | Description |
|---|---|
| `Authenticate` | Validate credentials and start a session |
| `VerifyMfa` | Complete an MFA challenge |
| `RefreshToken` | Rotate a refresh token |
| `RevokeSession` | End a session |
| `RegisterDevice` | Bind a new device |
| `StepUp` | Elevate a session for a sensitive operation |

## Events

| Publishes | Consumes |
|---|---|
| `identity.login.succeeded` | `user.registered` (create credentials) |
| `identity.login.failed` | |
| `identity.device.registered` | |
| `identity.session.revoked` | |

## Data

`credentials`, `sessions`, `refresh_tokens`, `devices`, `mfa_factors`, `outbox`, `inbox`.
