# Trk CLI

Take a [Trk](https://trk.hullop.com) card to your terminal, work it with
your own coding agent, and let the pull request find its way back to the
card.

```sh
trk login
trk card take BaHtsbhzVDa9 --branch
claude "Read .trk/cards/BaHtsbhzVDa9/card.md and its attachments, then do what the card asks."
git push -u origin HEAD && gh pr create --fill
```

The pull request, opened from `trk/card-BaHtsbhzVDa9`, attaches to its card
by itself: the card moves to review and shows the pull request, and moving
the card to done merges it.

> **Status:** in development. The commands below describe the first
> release; not all of them work yet.

## Install

```sh
brew install hullop/tap/trk                        # Homebrew
go install github.com/cirope/trk-cli/cmd/trk@latest # Go
```

Or download a binary for macOS, Linux or Windows from the
[releases](https://github.com/cirope/trk-cli/releases).

## Sign in

```sh
trk login
```

`trk` shows a link and opens it in your browser. Check the code matches,
pick the organization, and approve. No password or token is ever typed or
pasted, so it works the same over SSH and inside containers
(`--no-browser` skips opening the browser).

The session is kept in your system's keychain (macOS Keychain, Windows
Credential Manager, Secret Service on Linux), or in a private file in your
config directory when there is no keychain. `trk whoami` shows who you
are; `trk logout` revokes the session.

## Commands

| Command | What it does |
|---|---|
| `trk cards` | Your open cards across every board: overdue, this week, later, no date. |
| `trk card show <card>` | A card as you'd read it in Trk. `--markdown` prints the same document as the web app's Download. `--web` opens it in the browser. |
| `trk card pull <card>` | Writes the card and its files to `.trk/cards/<card>/`. |
| `trk card take <card>` | Makes you an owner, turns Trk's agent off for the card, moves it to In progress, pulls it, and prints the branch to use. `--branch` creates and switches to it. |
| `trk login`, `logout`, `whoami` | Sign in and out. |

`<card>` is a card id (`BaHtsbhzVDa9`) or its URL, as copied from the
browser. Run `trk <command> --help` for every flag and an example.

### With your own agent

`trk card take` turns Trk's agent off **before** moving the card, so the
move never starts it, and every step is safe to repeat. The card's
`card.md` ends with a "Bringing it back" section naming the branch, so the
agent knows too.

`trk card pull` keeps card content out of git: inside a repository it adds
`/.trk/` to `.git/info/exclude` (which is never committed). Card content
can include internal notes about customers; it is meant for whoever works
the card, not for the repository.

### For scripts and agents

- `--json` prints one JSON document on stdout and nothing else. Fields are
  only ever added, never renamed or removed. Errors under `--json` are a
  JSON object on stderr with `code` and `message`.
- Results go to stdout; progress and diagnostics go to stderr.
- Exit codes are stable:

  | Code | Meaning |
  |---|---|
  | 0 | Done |
  | 1 | Something failed (network, server error) |
  | 2 | Wrong usage (unknown flag, missing argument) |
  | 3 | Not found, or not yours |
  | 4 | Not signed in, or the session expired or was revoked |
  | 5 | Refused: understood and not allowed |

### Flags and environment

| Flag | Environment | |
|---|---|---|
| `--json` | | Machine-readable output |
| `--host <url>` | `TRK_HOST` | Another Trk install (default `https://trk.hullop.com`) |
| `--no-color` | `NO_COLOR` | No colours |
| `-q`, `--quiet` | | Only errors and the essential result |
| `-v`, `--verbose` | `TRK_DEBUG=1` | Requests and timings on stderr (never the token) |
| | `TRK_TOKEN` | Use this token instead of the stored session |

## Development

Requires Go (the version is pinned in `go.mod`). One command runs every
check CI runs: formatting, `go mod tidy`, vet, staticcheck, golangci-lint,
the tests with the race detector, govulncheck, and builds for macOS, Linux
and Windows.

```sh
make check
make build   # bin/trk
```

Linters and other tools are pinned in `tools/go.mod` and run with
`go tool`; nothing needs installing by hand. Tests never touch the network,
your keychain or your config.
