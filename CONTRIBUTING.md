# Contributing

## Workflow

1. Create a branch: `git checkout -b feat/ledger-postings`
2. Make your change, with tests.
3. Run `make lint test` before pushing.
4. Open a pull request against `main`. CI runs the same checks.

## Code guidelines

- **Go 1.26+**, formatted with `gofmt`, clean under `go vet`.
- Each service is its own module under `services/<name>` and is registered in `go.work` (`go work use ./services/<name>`).
- Services follow the hexagonal layout: `internal/domain` (no I/O), `internal/application` (use cases), `internal/ports` (interfaces), `internal/adapters` (postgres, grpc, kafka, http).
- **Money is never a float.** Use `packages/go-common/money`, which stores int64 minor units.
- Architectural changes come with an ADR in `docs/decisions/` (see the [template](docs/decisions/0000-template.md)).

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/) with the service as scope:

```
feat(ledger): reject unbalanced journal entries
fix(money): detect overflow on Sub with MinInt64
docs(adr): add 0016 idempotency keys
```

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
