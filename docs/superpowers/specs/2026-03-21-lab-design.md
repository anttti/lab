# lab — GitLab Merge Request TUI

A CLI tool with a TUI for managing GitLab merge requests, focused on processing unresolved comments and dispatching them to Claude Code for fixes.

## Architecture

Single Go binary (`lab`) with subcommands. Bubbletea for the TUI, Lipgloss for styling. All GitLab communication goes through the `glab` CLI tool (expected to be installed and authenticated). Persistence in SQLite with WAL mode.

### Commands

```
lab                          # launches the TUI (default)
lab add <local-path>         # registers a local git repo
lab remove <local-path>      # unregisters a repo
lab list                     # lists registered repos
lab sync                     # one-shot sync of all repos
lab sync --loop --interval N # sync repeatedly (used by daemon/launchd)
lab daemon start             # forks sync loop into background
lab daemon stop              # stops the background daemon
lab daemon status            # checks if daemon is running
lab daemon install           # generates and loads a launchd plist
lab daemon uninstall         # unloads and removes the plist
lab config                   # set username, sync interval, etc.
```

### Data Directory

`~/.config/lab/` containing:
- `lab.db` — SQLite database
- `daemon.pid` — PID file for manual daemon
- `daemon.log` — daemon log output

The launchd plist goes to `~/Library/LaunchAgents/com.lab.sync.plist`.

## Data Model

### repos

| Column         | Type      | Notes                              |
|----------------|-----------|------------------------------------|
| id             | INTEGER   | PRIMARY KEY                        |
| path           | TEXT      | UNIQUE, local filesystem path      |
| gitlab_url     | TEXT      | remote GitLab URL (from git remote)|
| project_id     | INTEGER   | GitLab project ID                  |
| name           | TEXT      | repo name                          |
| added_at       | TIMESTAMP |                                    |
| last_synced_at | TIMESTAMP |                                    |

### merge_requests

| Column          | Type      | Notes                             |
|-----------------|-----------|-----------------------------------|
| id              | INTEGER   | PRIMARY KEY                       |
| repo_id         | INTEGER   | FK → repos                        |
| iid             | INTEGER   | MR number within the project      |
| title           | TEXT      |                                   |
| author          | TEXT      | GitLab username                   |
| state           | TEXT      | opened, closed, merged            |
| source_branch   | TEXT      |                                   |
| target_branch   | TEXT      |                                   |
| web_url         | TEXT      |                                   |
| pipeline_status | TEXT      | success, failed, running, pending, null |
| updated_at      | TIMESTAMP | from GitLab                       |
| synced_at       | TIMESTAMP |                                   |

UNIQUE(repo_id, iid)

### mr_labels

| Column | Type    | Notes                |
|--------|---------|----------------------|
| mr_id  | INTEGER | FK → merge_requests  |
| label  | TEXT    |                      |

PRIMARY KEY(mr_id, label)

### comments

| Column        | Type      | Notes                              |
|---------------|-----------|------------------------------------|
| id            | INTEGER   | PRIMARY KEY                        |
| mr_id         | INTEGER   | FK → merge_requests                |
| discussion_id | TEXT      | GitLab discussion ID (groups threads) |
| note_id       | INTEGER   | GitLab note ID                     |
| author        | TEXT      |                                    |
| body          | TEXT      |                                    |
| file_path     | TEXT      | NULLABLE, null for general comments|
| old_line      | INTEGER   | NULLABLE                           |
| new_line      | INTEGER   | NULLABLE                           |
| resolved      | BOOLEAN   |                                    |
| created_at    | TIMESTAMP |                                    |
| synced_at     | TIMESTAMP |                                    |

UNIQUE(mr_id, note_id)

### config

| Column | Type | Notes       |
|--------|------|-------------|
| key    | TEXT | PRIMARY KEY |
| value  | TEXT |             |

Stores: username, sync_interval, active_repo_filter, active_user_filter, active_label_filters (comma-separated), etc.

## Sync Engine

All GitLab communication uses `glab` with the `-R <gitlab_url>` flag. No need to cd into repo directories.

### Sync flow per repo

1. Run `glab mr list -R <gitlab_url> --state opened --json` — upsert into `merge_requests` and `mr_labels`. Only open MRs are synced.
2. For each MR whose `updated_at` is newer than its `synced_at` (or has never been synced), run `glab mr note list <iid> -R <gitlab_url> --json` — upsert into `comments`. This avoids re-fetching notes for unchanged MRs.
3. Remove MRs from the DB that are no longer in the open set returned by GitLab. Remove orphaned comments accordingly.
4. Update `repos.last_synced_at`

`project_id` is parsed from the `glab mr list` JSON output on first sync after `lab add`.

### Sync modes

- **One-shot:** `lab sync` — runs once for all repos, exits
- **Loop:** `lab sync --loop --interval 5m` — repeats on a timer (used by daemon and launchd)
- **In-TUI:** background goroutine syncs periodically, sends a Bubbletea message to refresh the view

Repos sync sequentially to avoid hammering GitLab. Default interval: 5 minutes, configurable.

## TUI Layout

### MR List View (home screen)

```
┌─ lab ──────────────────────────────────────────────┐
│ Filters: [All repos ▾] [My MRs ▾] [Labels ▾]      │
│                                                     │
│  ● my-project !42  Fix auth timeout    @alice  3↩ ✓ │
│  ● my-project !38  Update deps         @me     1↩ ✗ │
│  ● other-repo !15  Add logging         @bob    5↩ ⟳ │
│  ● other-repo !12  Refactor parser     @me     0↩ ✓ │
│                                                     │
│ j/k navigate  enter/l select  f filter  q quit      │
└─────────────────────────────────────────────────────┘
```

- **Vim-style keybindings** throughout: `j`/`k` up/down, `l`/`enter` select/drill in, `h`/`b` back, `g`/`G` top/bottom. Arrow keys also work as fallback.
- Pipeline status: `✓` passed, `✗` failed, `⟳` running, `—` no pipeline
- Unresolved comment count derived from the `comments` table at render time
- Filter toggles: repo (all / specific), user (all / just me), labels, unresolved-only
- Only MRs with state "opened" shown by default

### Filter Overlay

Pressing `f` opens a filter overlay. `tab`/`shift-tab` cycles between filter groups.

- **Repo:** Scrollable list with "All repos" at top, followed by all registered repos. `j`/`k` to navigate, `enter` to select, `esc` to cancel. Single-select.
- **User:** Toggle between "All" and "Only me" with `space`. Single-select.
- **Labels:** Multi-select list of all labels found on synced MRs (auto-populated from `mr_labels` table, no manual management). `j`/`k` to navigate, `space` to toggle. "No filter" at top. MR must have at least one selected label to be shown.

`esc` from the group level closes the overlay and applies filters. Active filter state is persisted in the `config` table so it survives restarts.

Status bar shows active filters: e.g. `Filters: other-repo | Only me | urgent, review-needed`

### MR Detail View

```
┌─ my-project !42 — Fix auth timeout ────────────────┐
│                                                     │
│ ▸ src/auth/handler.go:45 (2 notes, unresolved)      │
│   "This timeout should be configurable..."          │
│                                                     │
│ ▸ src/auth/config.go:12 (1 note, unresolved)        │
│   "Missing validation for negative values"          │
│                                                     │
│ ▸ General: (1 note, resolved)                       │
│   "LGTM on the overall approach"                    │
│                                                     │
│ ↑↓ navigate  enter view thread  r sync  b back      │
└─────────────────────────────────────────────────────┘
```

- Comments grouped by discussion thread
- Shows file path, line number, resolved status, first line of comment
- Unresolved threads shown first
- Pressing `r` triggers a sync for this specific MR (re-fetches notes via `glab`), with a spinner while syncing

### Thread View

```
┌─ src/auth/handler.go:45 ───────────────────────────┐
│                                                     │
│ @reviewer (2h ago):                                 │
│ This timeout should be configurable, not            │
│ hardcoded to 30s. See config pattern in             │
│ src/auth/config.go.                                 │
│                                                     │
│ @author (1h ago):                                   │
│ Good point, will fix.                               │
│                                                     │
│ c launch Claude Code  b back                        │
└─────────────────────────────────────────────────────┘
```

## Claude Code Integration

When the user presses `c` on a thread:

1. **Prompt choice:** TUI shows "Send as-is or augment? (a/s)"
2. **If augment:** Opens `$EDITOR` with the pre-built prompt. User edits, saves, exits.
3. **Prompt format:**

```
File: src/auth/handler.go (line 45)
Full path: /Users/me/projects/my-project/src/auth/handler.go

--- Comment thread ---
@reviewer:
This timeout should be configurable, not hardcoded to 30s.
See config pattern in src/auth/config.go.

@author:
Good point, will fix.
--- End thread ---

Verify this issue exists and then fix it.
```

4. **Launch:** Write prompt to a temp file. Open a new terminal window running `cd <repo-path> && claude --prompt-file <tmpfile>`.
5. **Terminal detection:** Check `$TERM_PROGRAM` to detect iTerm2 vs Terminal.app. Use AppleScript to open a new window/tab. Fallback to `open -a Terminal.app`.

The TUI resumes immediately since Claude Code runs in a separate window.

## Daemon & Background Sync

### Manual daemon

- `lab daemon start` re-executes itself as `lab sync --loop --interval <configured>` in background
- Writes PID to `~/.config/lab/daemon.pid`
- Logs to `~/.config/lab/daemon.log`
- `lab daemon stop` reads PID file, sends SIGTERM
- `lab daemon status` checks if the PID is alive

### launchd integration

- `lab daemon install` generates a plist at `~/Library/LaunchAgents/com.lab.sync.plist` running `lab sync --loop --interval <configured>`
- Loads via `launchctl load`
- `lab daemon uninstall` unloads and removes the plist
- When using launchd, the PID file is not used — launchd manages the process

### SQLite concurrency

WAL mode enabled for concurrent access. The sync engine uses short write transactions (upsert a batch, commit) to avoid blocking the TUI.

## Scope & Limitations

- **macOS only** for now — launchd, AppleScript terminal launching, `open -a` are macOS-specific. Cross-platform support (Linux systemd, etc.) is a future consideration.
- **Read-only interaction with GitLab** — the TUI does not reply to comments or resolve threads. It dispatches to Claude Code for fixes.
- **TUI scrolling** handled by the bubbles list component for long MR lists.

## Error Handling

- **Repo path deleted/moved:** Sync skips with warning. TUI shows warning icon. `lab list` shows status.
- **`glab` not installed/authenticated:** Verified on `lab add`. Clear error message if missing.
- **Network offline:** Sync fails gracefully, keeps existing data, retries next cycle. TUI works with stale data.
- **GitLab rate limiting:** Back off, retry next sync cycle.
- **No diff notes on MR:** Shown under "General" section, no file/line prefix. Can still be sent to Claude Code.
- **Claude Code not installed:** Check `claude` is on PATH when pressing `c`. Show error if not.
- **DB migrations:** Schema version checked on startup, migrations run if needed.

## Dependencies

- [bubbletea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [lipgloss](https://github.com/charmbracelet/lipgloss) — TUI styling
- [bubbles](https://github.com/charmbracelet/bubbles) — TUI components (list, viewport, text input)
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — pure Go SQLite driver (no CGO)
- [cobra](https://github.com/spf13/cobra) — CLI framework
