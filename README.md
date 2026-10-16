# lgtm

**Looks good to me** – a fast, distraction-free terminal UI for *reading and commenting on* code changes, built for agentic workflows.

When coding agents (Claude Code, pi, …) write most of your changes, your job shifts from writing to understanding and judging. `lgtm` gives you a vim-style reading experience for the diff, lets you leave categorized comments inline, and hands them back to the agent as a ready-to-paste prompt. In the next round it shows you only what changed since your last review.

- **Whole-file diffs**: full files with highlighted changes and collapsed unchanged regions, so context is one fold away (unified view is a toggle).
- **Zen mode**: the screen belongs to the code. Just a quiet status line; file list and pickers are overlays.
- **vim/nvim navigation**: hunks, files and comments via `]`/`[`, folds via `z`, search, project-wide grep, heuristic go-to-definition, jumplist.
- **Comments for agents**: `must` / `should` / `nit` / `question`, written to `.lgtm/review.md` instantly and copyable as a prompt.
- **Review rounds**: an interdiff shows only what changed since your last review; old comments stay visible and can be ticked off.
- **Approval via exit code**: usable as a blocking step in agent scripts.
- Tokyo Night themes (auto light/dark), intra-line highlighting, smooth scrolling, minimap scrollbar.
- Pure Go: no cgo, no `git` binary needed. Works in linked worktrees.

![lgtm demo: review overview, commenting a hunk and jumping to another file](docs/demo/demo.gif)

The demo is recorded with [VHS](https://github.com/charmbracelet/vhs) from [`docs/demo/demo.tape`](docs/demo/demo.tape) on a sample repository created by [`docs/demo/setup.sh`](docs/demo/setup.sh). Re-render it with `vhs docs/demo/demo.tape` from the repository root (needs `vhs`, `ttyd`, `ffmpeg` and the Iosevka font).

## Installation

```sh
go install github.com/sdillen/lgtm@latest
```

Or build from a checkout:

```sh
go build -o lgtm .
```

With Nix, the repository ships a `package.nix` (`pkgs.callPackage ./package.nix { }`).

Check the installation with `lgtm --version` and `lgtm --help`.

## Quick start

Inside a git repository:

```sh
lgtm                # everything since the branch diverged from the default branch,
                    # including commits and uncommitted changes
lgtm abc1234        # a single commit
lgtm main..feature  # an explicit range
```

The default branch (`main`, `master`, or the remote's HEAD) is detected automatically.

Typical agent loop:

1. The agent changes code (possibly in its own worktree).
2. You run `lgtm`, read, and press `c` to comment.
3. Press `Y` to copy all open comments as a prompt and paste it into the agent session. The agent can also read `.lgtm/review.md` directly.
4. After the agent has worked, run `lgtm` again and press `<space>i` to see only the interdiff since your last review. Comments of the earlier round keep their colour and are labelled `Runde N ·`; tick off what the agent did with `x`. Done comments are dimmed, struck through and marked `✓`, and the status line counts them (`1/2 erledigt`).
5. Happy? Run `:lgtm` to approve. This ends the review and clears its state (`.lgtm/` and the snapshot refs, like `lgtm clear`), so the next run starts a fresh review. With open comments `:lgtm` asks first ("N offene Kommentare werden verworfen"); run it again to confirm.

## Key bindings

Press `?` inside the app for the full, current list. After `<space>`, `]`, `[`, `g` or `z`, a which-key popup shows the possible follow-up keys. Counts work as in vim (`5j`, `3]c`).

### Movement

| Key | Action |
| --- | --- |
| `j` / `k` | line down / up |
| `h` / `l`, `zh` / `zl` | scroll horizontally |
| `<C-d>` / `<C-u>` | half page down / up |
| `gg` / `G` | first / last line (with count: line N) |
| `zz` / `zt` | center / top the cursor line |
| `]c` / `[c` | next / previous hunk |
| `]f` / `[f` | next / previous file |
| `]k` / `[k` | next / previous comment |
| `zo` `zc` `zR` `zM` | open / close fold, open / close all folds |
| `<C-o>` / `<C-i>` | jump back / forward |

### Search and navigation

| Key | Action |
| --- | --- |
| `/`, `n`, `N` | search in the file |
| `*` / `#` | next / previous occurrence of the word under the cursor |
| `gr` | search the word in the whole repo |
| `gd` | go to definition (heuristic, no LSP) |
| `<space>p` | pick a changed file |
| `<space>f` | pick any file of the repo (read-only, current state) |
| `<space>d` | toggle the file drawer |

### Reviewing and commenting

| Key | Action |
| --- | --- |
| `c` | comment the line (or edit the comment under the cursor) |
| `V` | select lines, then `c` to comment the range |
| `C` | comment the whole file |
| `<space>c` | comment the whole review |
| `dd` | delete the comment (press twice to confirm) |
| `x` | mark the comment as done / open again |
| `Y` | copy all open comments as a prompt to the clipboard |
| `yc` | copy the comment under the cursor |
| `<space>r` | toggle read status of the file |
| `<space>i` | toggle interdiff since the last round |
| `u` | toggle unified view |
| `R` | reload after files changed |
| `q` | quit |

In the comment float: `<Tab>` cycles the category (`must` → `should` → `nit` → `question`), `<C-s>` saves, `<Esc><Esc>` discards.

### Command line

| Command | Effect |
| --- | --- |
| `:lgtm` | approve the review, clear its state and exit with code 0 (asks first with open comments) |
| `:q` / `:q!` | quit (warns about open comments) / quit anyway |
| `:theme [name]` | switch theme |
| `:set <option>` | vim-style options `unified`, `sidebar`, `wrap`, `ignorews`: `:set wrap` on, `:set nowrap` off, `:set wrap!` toggle, `:set wrap?` query |

Files are marked as *read* once you have scrolled to their end; the mark resets when the file changes in a later round. Generated files, lockfiles and vendored code are collapsed and sorted to the end. Renames show as `old → new`, binary files as metadata only.

## How comments reach the agent

Every comment is written immediately to `.lgtm/review.md` in your repository, grouped by file, with `file:lines`, the quoted code and the category. Only open comments are included. `.lgtm/` is added to `.git/info/exclude` automatically, so it is never committed.

`Y` renders the open comments through a prompt template (`must` first; `question` marked as "answer only, change nothing") and copies it to the clipboard. This works over SSH and inside multiplexers (OSC 52, with fallback to `wl-copy`, `xclip` or `pbcopy`).

To customize the prompt, print the built-in template and save your own:

```sh
lgtm --print-template > ~/.config/lgtm/prompt.template   # global
lgtm --print-template > .lgtm/prompt.template            # per project (wins over global)
```

If files change while you review, the status line tells you; nothing jumps under your cursor until you press `R`. Comments are re-anchored via their quoted code; if the code is gone they are marked *orphaned* rather than deleted.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | approved with `:lgtm`, or quit without open comments |
| `1` | quit with open comments |
| `2` | error |

## Configuration

Configuration lives in `$XDG_CONFIG_HOME/lgtm/` (usually `~/.config/lgtm/`). All settings are optional.

```toml
# ~/.config/lgtm/config.toml
theme = "tokyo-night-night"   # default: auto-detect light/dark
leader = "<space>"            # default: <space>
smooth_scroll = true          # set to false to disable animations
icons = false                 # Nerd Font icons in the file list

[keys]                        # rebind actions; an empty list unbinds
next-hunk = ["]c", "<C-j>"]
down = ["j", "<down>"]
```

Action names are the ones listed in the help (`?`), e.g. `down`, `next-hunk`, `toggle-interdiff`. The which-key popup and help reflect your bindings.

### Themes

`tokyo-night-day` and `tokyo-night-night` are built in; `--theme <name>` or `:theme` overrides the config. Custom themes are TOML files in `~/.config/lgtm/themes/`, defining semantic tokens (`[ui]`, `[diff]`, `[comment]`, `[syntax]`); missing tokens fall back to the built-in dark theme.

```toml
# ~/.config/lgtm/themes/my-theme.toml
name = "my-theme"

[ui]
purple = "#bb9af7"            # accents that tell elements apart
orange = "#ff9e64"

[comment]
must = "#f7768e"
nit = "#7dcfff"

[syntax]
base = "tokyonight-night"     # any Chroma style

[syntax.overrides]
Keyword = "#bb9af7"
Comment = { fg = "#51597d", italic = true }
```

Terminals without truecolor fall back to 256 colors automatically.

## Resetting a review

```sh
lgtm clear        # asks first
lgtm clear --yes  # required without a terminal
```

This deletes `.lgtm/` and the snapshot refs under `refs/lgtm/`.

## What lgtm does not do

`lgtm` is for reading, not for operating git or editing code: no staging, committing, checkout or push; no side-by-side view; no LSP; no remote PR integration. See [`docs/prd.md`](docs/prd.md) for the full scope and design decisions.

## Development

The repository uses [devenv](https://devenv.sh) with `gofmt`, `go test` and `golangci-lint` hooks.

```sh
go test ./...
go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD)" -o lgtm .
```
