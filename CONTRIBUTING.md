# Contributing

Thanks for taking the time to contribute!

## Workflow

1. Fork the repository and create a branch from `main`:
   ```sh
   git checkout -b feature/short-description
   ```
2. Make your changes, keeping commits focused and descriptive.
3. Run formatting, vetting, and tests before pushing:
   ```sh
   gofmt -l .
   go vet ./...
   go test ./...
   ```
4. Push your branch and open a pull request against `main` using the
   provided PR template.
5. Ensure CI passes and address any review feedback.

## Commit Messages

Use short, imperative-mood summaries (e.g. "Add health check endpoint",
not "Added" or "Adding"). Reference related issues where relevant
(e.g. `Fixes #12`).

## Code Style

- Format all code with `gofmt` (or `goimports`) before committing.
- Follow standard Go idioms — see [Effective Go](https://go.dev/doc/effective_go).
- Keep exported identifiers documented with Go doc comments.
- Place unit tests alongside the code they test (`foo_test.go` next to
  `foo.go`); place broader integration tests under `tests/`.

## Reporting Issues

Use the bug report or feature request templates under
[.github/ISSUE_TEMPLATE](.github/ISSUE_TEMPLATE) when opening an issue.
