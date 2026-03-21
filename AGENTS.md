# Agents

Instructions for AI coding agents working on this project.

## Project

Go CLI/TUI tool for managing GitLab merge requests. See [README.md](README.md) for what it does and [ARCHITECTURE.md](ARCHITECTURE.md) for how it's built.

## Building and testing

```bash
go build -o lab .        # Build
go test ./... -v         # Run all tests
go vet ./...             # Lint
```

Tests use real SQLite databases in temp directories (`t.TempDir()`). The sync engine tests use a mock `GlabClient` interface — no network calls needed.

## Key conventions

- **Package layout**: Commands in `cmd/`, domain logic in `internal/`. Each `internal/` package has a single responsibility.
- **Database access**: All SQL goes through `internal/db/`. Use `sqlx` named queries and struct scanning. Upserts use `INSERT ... ON CONFLICT DO UPDATE`.
- **GitLab communication**: Always through `glab` CLI (never direct HTTP). Use `-R <url>` flag, not `cd` into repos. MR listing via `glab mr list -F json`, discussions via `glab api`.
- **TUI pattern**: Bubbletea models with typed messages. Sub-models get a `*Model` pointer for view switching. Async data loading via `tea.Cmd` returning message types. Vim-style keybindings defined in `keys.go`.
- **Error handling**: Sync errors are logged and skipped (don't fail the whole sync for one bad repo). CLI errors use Cobra's `RunE` pattern. TUI shows errors inline.
- **Platform**: macOS-specific code guarded by build tags (`daemon_darwin.go` / `daemon_notdarwin.go`).

## Spec and plan

The design spec is at `docs/superpowers/specs/2026-03-21-lab-design.md` and the implementation plan at `docs/superpowers/plans/2026-03-21-lab-implementation.md`. Refer to these for the original intent behind design decisions.
