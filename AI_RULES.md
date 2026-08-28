# AI Assistant Rules

## Git Workflow
- **NEVER** use `git add .` or `git add -A` under any circumstances.
- **ALWAYS** explicitly list the files that are being staged (e.g. `git add path/to/file1.go path/to/file2.go`).
- This prevents accidentally committing unintended files, test files, temporary databases, and IDE generated files into the repository.
- Double-check the list of modified files before committing using `git status` or `git diff --name-only`.

## Build Workflow
- **ALWAYS** prefer `make` (e.g. `make dev` or `make`) instead of `go build`.
- The Makefile configures fast CGO optimizations (`-O1`) and faster linkers (`mold`), preventing long compilation times for gotk4 bindings.
