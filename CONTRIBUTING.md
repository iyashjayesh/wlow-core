# Contributing to wlow

## Getting started

1. Fork the repository and create a branch from `main`.
2. Follow the [setup guide](docs/setup.md) to build locally.
3. Make your changes, run `make fmt vet test-short` before opening a PR.

## What belongs in a PR

- **Bug fixes**: include a test that would have caught the regression.
- **Features**: open an issue first to discuss scope and design before investing time coding.
- **Docs**: welcome without a prior issue.
- **Refactors**: keep them small and scoped to a single concern.

## Code standards

- Follow Go conventions (`gofmt`, `go vet`, `golangci-lint`).
- All error returns must be explicitly handled or annotated.
- Functions must stay under 60 lines; decompose otherwise.
- Assertions or guard clauses near function entry for preconditions.
- No new globals without documented synchronization.

## Commit style

Use short imperative sentences:
- `fix: nil check in artifact resolver`
- `feat: add WASM processor support`
- `docs: update CLI reference for snapshot command`

## Security

Do not open public issues for security vulnerabilities. See [SECURITY.md](SECURITY.md).

## License

By contributing you agree that your contributions will be licensed under the [Apache-2.0 license](LICENSE).
