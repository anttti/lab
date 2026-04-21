# lab

A terminal UI for managing GitLab merge requests, focused on surfacing unresolved comments and dispatching them to [Claude Code](https://claude.ai/claude-code) for fixes.

## What it does

- Browse open merge requests across multiple GitLab repositories
- View unresolved comment threads with file paths and line numbers
- Filter MRs by repository, author, and labels
- Send comment threads to Claude Code in a new terminal window, with the file context pre-filled
- Sync MR data in the background via a daemon or launchd

## Requirements

- **Go 1.25+** (for building)
- **[glab](https://gitlab.com/gitlab-org/cli)** CLI tool, installed and authenticated (`glab auth login`)
- **macOS** (daemon/launchd features and terminal launching use macOS-specific APIs)
- **[Claude Code](https://claude.ai/claude-code)** CLI (`claude` on PATH) for the code fix workflow
- **[`task`](https://taskfile.dev/)** for task running

## Installation

Build from source:

```bash
task build
```

## Quick start

```bash
# Set your GitLab username (used for "Only me" filter)
lab config set username your-gitlab-username

# Register a local repo (must have a GitLab remote)
lab add /path/to/your/gitlab/repo

# Sync MR data from GitLab
lab sync

# Launch the TUI
lab
```

## Usage

### TUI

Running `lab` with no arguments launches the TUI.

**MR List** (home screen) shows all open MRs with pipeline status and unresolved comment count. **MR Detail** shows comment threads grouped by discussion. **Thread View** shows the full conversation and lets you launch Claude Code.

Navigation is vim-style:

| Key | Action |
|---|---|
| `j` / `k` | Move down / up |
| `l` / `Enter` | Select / drill in |
| `h` / `b` | Go back |
| `g` / `G` | Jump to top / bottom |
| `f` | Open filter overlay |
| `r` | Sync current MR (in detail view) |
| `c` | Launch Claude Code (in thread view) |
| `q` | Quit |

Arrow keys also work.

**Filters** (`f`): filter by repository (single-select), user (all or just you), and labels (multi-select, auto-populated from synced MRs). Filter state persists across restarts.

**Claude Code** (`c` in thread view): choose to send the comment thread as-is (`s`) or augment it in your `$EDITOR` first (`a`). The prompt includes the file path, line number, full comment thread, and the instruction "Verify this issue exists and then fix it." Claude Code opens in a new terminal window in the repo's working directory.

### CLI commands

```bash
lab                          # Launch TUI
lab --install                # Install the sync daemon (10 min interval, notifies via terminal-notifier)
lab add <path>               # Register a local git repo
lab remove <path>            # Unregister a repo
lab list                     # List registered repos
lab sync                     # One-shot sync
lab sync --loop --interval 5m  # Continuous sync
lab config set <key> <value> # Set config (e.g., username, sync_interval)
lab config get <key>         # Get config value
lab daemon start             # Start background sync daemon
lab daemon stop              # Stop daemon
lab daemon status            # Check if daemon is running
lab daemon install           # Install as macOS launchd service
lab daemon uninstall         # Remove launchd service
```

### Background sync

The daemon keeps your MR data fresh when the TUI isn't running:

```bash
# One-shot install: sets 10 min sync interval + loads launchd agent.
lab --install

# Manual daemon (stops when you stop it)
lab daemon start
lab daemon stop

# launchd service (starts on login, restarts on crash)
lab daemon install
lab daemon uninstall
```

The sync interval defaults to 5 minutes (10 minutes if installed via `lab
--install`). See **Configuration** below for how to change it.

The TUI also syncs in the background while running.

### Desktop notifications

When the daemon is running in loop mode and [terminal-notifier](https://github.com/julienXX/terminal-notifier)
is on `PATH` (`brew install terminal-notifier`), lab posts a macOS notification
for each change it detects on one of your MRs between syncs. Clicking the
notification opens the MR web URL.

## Configuration

`lab --install` writes a JSON file at `~/.config/lab/lab.json` with default
values. You can also create it by hand. Fields that are omitted fall back to
the defaults shown below.

```json
{
  "sync_interval": "10m",
  "username": "your-gitlab-username",
  "notifications": {
    "new_comment": true,
    "pipeline_failed": true,
    "approved": true,
    "mr_merged": true,
    "new_review_request": true,
    "rereview_request": true
  }
}
```

- **`sync_interval`** — how often the background daemon pulls fresh data
  (any Go `time.ParseDuration` string: `30s`, `5m`, `1h`, …). The launchd
  plist reads this value at `lab --install` time — rerun `lab --install`
  after changing it to reload the agent.
- **`username`** — your GitLab username. Used to determine which MRs are
  yours (for authored-by-you triggers) and when you are a reviewer.
- **`notifications`** — per-trigger toggles. Every trigger is on by default:
  - `new_comment` — a comment appears on one of your MRs.
  - `pipeline_failed` — one of your MRs' pipelines transitions to `failed`
    or `canceled`.
  - `approved` — one of your MRs receives its first approval.
  - `mr_merged` — one of your MRs was merged.
  - `new_review_request` — you were added as a reviewer on someone else's MR.
  - `rereview_request` — a review was re-requested (your reviewer state
    reset to `unreviewed`). Requires GitLab 15+ which exposes
    `reviewer_state`.

For backwards compatibility, `lab config set sync_interval …` /
`lab config set username …` still work; they write to the SQLite `config`
table and are used as a fallback when the same key is missing from
`lab.json`.

## Data storage

All data is stored in `~/.config/lab/`:

- `lab.json` — user configuration (sync interval, username, notification toggles)
- `lab.db` — SQLite database (MRs, comments, config)
- `daemon.pid` — PID file for the manual daemon
- `daemon.log` — Daemon log output

The launchd plist is stored at `~/Library/LaunchAgents/com.lab.sync.plist`.
