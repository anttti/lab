# Architecture

## Overview

Lab is a single Go binary with Cobra subcommands. The default command launches a Bubbletea TUI. Data is persisted in SQLite. All GitLab communication goes through the `glab` CLI tool.

```
┌─────────────────────────────────────────────────┐
│                    cmd/                          │
│  root  add  remove  list  sync  config  daemon  │
└────────┬────────────────────────────────────────┘
         │
    ┌────┴────┐
    │         │
┌───▼───┐ ┌──▼──────────────────────────────────┐
│  tui  │ │            internal/                 │
│       │ │  ┌──────┐  ┌──────┐  ┌───────────┐  │
│ model │ │  │ sync │──│ glab │  │  daemon    │  │
│ views │ │  └──┬───┘  └──────┘  └───────────┘  │
│ keys  │ │     │                                │
│ style │ │  ┌──▼──┐  ┌────────┐                 │
│       │ │  │ db  │  │ claude │                 │
│       │─┤  └─────┘  └────────┘                 │
└───────┘ └──────────────────────────────────────┘
```

## Packages

### `cmd/`

Cobra command definitions. Each file registers one command in `init()`. The root command opens the DB and launches the TUI. Helper functions `dataDir()` and `openDB()` are shared across commands.

Platform-specific files: `daemon_darwin.go` (launchd install/uninstall) and `daemon_notdarwin.go` (stubs).

### `internal/db/`

SQLite persistence layer using `jmoiron/sqlx` over `modernc.org/sqlite` (pure Go, no CGO). WAL mode is enabled via connection string pragmas for concurrent read/write access.

**Schema** (7 tables):
- `repos` — registered repositories with local path and GitLab URL
- `merge_requests` — MR metadata, UNIQUE on (repo_id, iid)
- `mr_labels` — many-to-many join table for MR labels
- `mr_reviewers` — many-to-many join table for MR reviewers with review state
- `comments` — discussion notes with optional file position, UNIQUE on (mr_id, note_id)
- `config` — key-value store for filter state (and legacy username/sync_interval)
- `thread_reads` — read receipts per discussion thread

Migrations use `CREATE TABLE IF NOT EXISTS`. Schema changes should add new migration statements.

**Key patterns:**
- Upsert via `INSERT ... ON CONFLICT DO UPDATE`
- `MRFilter` struct for dynamic query building (repo, author, labels)
- `ListThreads` groups comments by `discussion_id` into `Thread` structs
- Stale record deletion by diffing keep-lists against existing records

### `internal/glab/`

Thin wrapper that shells out to the `glab` CLI and parses JSON responses. Two communication paths:

1. `glab mr list -R <url> -F json` — for listing MRs (returns MR metadata, labels, author)
2. `glab api projects/:id/merge_requests/:iid/discussions` — for fetching discussion threads with file position data (the `glab mr` subcommands don't expose this)

Pipeline status requires a separate `glab api` call to the MR detail endpoint to get `head_pipeline`.

The `Client` struct satisfies the `sync.GlabClient` interface via Go structural typing, enabling mock-based testing of the sync engine.

### `internal/sync/`

Orchestrates data flow from GitLab to SQLite.

**`GlabClient` interface** — the only abstraction boundary in the sync layer. Tests use a `mockGlab` that returns canned responses.

**Sync flow** (`SyncRepo`):
1. Verify repo path exists on disk
2. Fetch open MRs via glab
3. For each MR: fetch pipeline status, upsert MR + labels
4. **Incremental optimization**: only fetch discussions if the MR's `updated_at` is newer than its last `synced_at`
5. Delete MRs no longer in the open set
6. Update repo's `last_synced_at`

**Discussion sync** (`syncDiscussions`):
1. Fetch discussions from GitLab API
2. Filter out system notes (e.g., "assigned to @user")
3. Extract file position from `note.Position` (new_path, old_line, new_line)
4. Upsert comments, delete stale ones

### `internal/tui/`

Bubbletea application with four views managed by a root `Model` that routes `Update`/`View` calls.

**View hierarchy:**
```
MR List ──select──▶ MR Detail ──select──▶ Thread ──c──▶ Claude Choice
   │                    │                    │
   ▼ f                  ▼ r                  ▼ h/b
Filter Overlay      Sync this MR         Back to Detail
```

**State management**: Sub-models receive a `*Model` pointer to switch views. Data is loaded asynchronously via `tea.Cmd` functions that return typed messages (e.g., `mrsLoadedMsg`, `threadsLoadedMsg`).

**Background sync**: A `tea.Tick` fires every 5 minutes, triggering `SyncAll()` in a Bubbletea command. On completion, `bgSyncDoneMsg` refreshes the MR list if it's the active view.

**Filter persistence**: Active filters are stored in the `config` table as `active_repo_filter`, `active_user_filter`, and `active_label_filters` (comma-separated). Labels are auto-populated from `mr_labels` — no manual label management needed.

### `internal/claude/`

Builds prompts from comment threads and launches Claude Code in a new terminal.

**Prompt format:**
```
File: <path> (line N)
Full path: /absolute/path/to/file

--- Comment thread ---
@author:
Comment body...
--- End thread ---

Verify this issue exists and then fix it.
```

**Terminal launching**: Uses AppleScript via `osascript` to open a new Terminal.app or iTerm2 window (detected via `$TERM_PROGRAM`). The command is `cd <repo> && claude --prompt-file <tmpfile>`.

**Augment flow**: The TUI uses `tea.ExecProcess` to suspend itself and open `$EDITOR` on a temp file containing the prompt. After editing, the file is read back and sent to Claude Code.

### `internal/daemon/`

Process management for background sync.

**Manual daemon** (`lab daemon start`): Re-executes the binary as `lab sync --loop` in a detached session (`Setsid: true`). PID stored in `~/.config/lab/daemon.pid`. Stopped via SIGTERM.

**launchd** (`lab daemon install`): Generates a plist via `text/template` and loads it with `launchctl`. The plist sets `KeepAlive: true` and `RunAtLoad: true`.

## Testing

Tests are concentrated in packages with meaningful logic to test:

- `internal/db/` — 22 tests covering all CRUD operations, filtering, stale deletion
- `internal/sync/` — 5 tests using mock glab client (create, delete stale, skip missing path, filter system notes)
- `internal/claude/` — 2 tests for prompt building
- `internal/daemon/` — 5 tests for PID management and plist generation
- `internal/glab/` — 1 test (install check, skips if glab not present)

TUI code is not unit tested — it's verified by building and manual interaction.

## Key design decisions

- **glab CLI over GitLab API client**: Avoids managing OAuth tokens, leverages the user's existing `glab auth`. The `-R` flag means we don't need to cd into repo directories.
- **SQLite with WAL**: Allows the daemon and TUI to access the DB concurrently without locking issues.
- **Pure Go SQLite** (`modernc.org/sqlite`): No CGO dependency, single binary distribution.
- **Incremental sync**: Checking `updated_at` vs `synced_at` avoids re-fetching discussions for unchanged MRs, reducing API calls significantly for repos with many open MRs.
- **macOS-only for now**: launchd, AppleScript terminal launching, and `Setsid` are macOS/Unix-specific. Cross-platform support would need systemd, different terminal launching, etc.
