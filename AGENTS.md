# AI Assistant Rules

## Git Workflow
- **NEVER** use `git add .` or `git add -A` under any circumstances.
- **ALWAYS** explicitly list the files that are being staged (e.g. `git add path/to/file1.go path/to/file2.go`).
- This prevents accidentally committing unintended files, test files, temporary databases, and IDE generated files into the repository.
- Double-check the list of modified files before committing using `git status` or `git diff --name-only`.

## Build Workflow
- **ALWAYS** prefer `make` (e.g. `make dev` or `make`) instead of `go build`.
- The Makefile configures fast CGO optimizations (`-O1`) and faster linkers (`mold`), preventing long compilation times for gotk4 bindings.

## Test Workflow
- **NEVER** run the full test suite (`go test ./...`). It takes way too long because of heavy CGO/gotk4 compilation across all UI packages.
- **ALWAYS** prefer `make` commands (such as `make test`) or test only specific target packages/functions (e.g. `go test -v ./internal/database/...` or `go test -v -run TestName ./pkg/...`).

## Toolset
- **ALWAYS** execute build, test, and development tools inside `nix-shell` (e.g. `nix-shell --run "make dev"` or `nix-shell --run "make test"`). Many tools and system libraries are only available inside the nix-shell environment.
