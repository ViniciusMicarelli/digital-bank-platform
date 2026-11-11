# Contributing

Thanks for your interest in contributing! This guide covers the conventions used across the repository.

## Workflow

1. Create a branch from `main` using the pattern `<type>/<short-description>`, for example `feat/pix-qr-code` or `fix/ledger-rounding`.
2. Make your changes, with tests.
3. Make sure `make lint test` passes.
4. Open a Pull Request that describes **what** changed and **why**.

## Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <description>

feat(pix): add dynamic QR code generation
fix(ledger): reject entries with unbalanced postings
docs(adr): add ADR-0007 transactional outbox
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`, `perf`, `build`.
Scope: the service or package name (`pix`, `ledger`, `go-messaging`, ...).

## Go Code Guidelines

- Follow [Effective Go](https://go.dev/doc/effective_go) and the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
- Code must pass `gofmt`, `go vet` and `golangci-lint`.
- **Never use `float` for money.** Use the `Money` type from `packages/go-common` (`int64` minor units plus currency).
- Pass `context.Context` as the first argument of every function that does I/O.
- Wrap errors with context (`fmt.Errorf("creating account: %w", err)`) and use typed errors from `packages/go-errors` at boundaries.
- The `domain` layer must not import adapters, frameworks or infrastructure packages.
- Use structured logging with `packages/go-logger`. Never log sensitive data (PAN, CVV, passwords, tokens, full CPF).

## Tests

| Type | Location | Tooling |
|---|---|---|
| Unit | Next to the code (`*_test.go`) | `testing`, table-driven tests |
| Integration | `services/<name>/test/integration` | Testcontainers (PostgreSQL, Kafka) |
| Contract | `contracts/` | `buf breaking`, schema validation |
| End-to-end | `tests/e2e` | Black-box tests through the API Gateway |

Domain logic should be well covered by unit tests.

## Contracts

- Any change to `contracts/` must be **backward compatible**. CI runs `buf breaking`.
- A breaking change requires a new version (`v2`) of the API or event.

## Architecture Decisions

Significant technical decisions must be recorded as an ADR in [`docs/decisions`](docs/decisions/README.md).
