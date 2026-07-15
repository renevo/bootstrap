# Contributing

Thank you for considering a contribution to Bootstrap.

## Before You Start

- Search existing issues and pull requests before opening a new one.
- Open an issue for substantial changes so the approach can be discussed first.
- Keep changes focused and avoid unrelated refactoring.

## Development

Bootstrap requires Go 1.26 or later.

Install [`goimports`](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) and
[`golangci-lint` v2](https://golangci-lint.run/docs/welcome/install/) before
submitting changes.

1. Fork the repository and create a branch from `main`.
2. Make your changes and add tests where appropriate.
3. Format and verify the code:

   ```bash
   goimports -w .
   golangci-lint run
   go test ./...
   ```

4. Update documentation when behavior or public APIs change.
5. Open a pull request describing the problem, the solution, and how it was
   tested.

## Reporting Bugs

Include the Go version, operating system, relevant configuration, steps to
reproduce the problem, expected behavior, and actual behavior. Remove secrets
and other sensitive information from logs and examples.

## Code of Conduct

All contributors must follow the [Code of Conduct](CODE_OF_CONDUCT.md).
