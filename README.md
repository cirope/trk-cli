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

> **Status:** ready for its first release; not published yet.

## Install

Every release has binaries for macOS, Linux and Windows on amd64 and
arm64. The commands below are for amd64 (Intel/AMD); on ARM, write
`arm64` instead.

### macOS

```sh
brew install cirope/tap/trk
```

### Linux

With Homebrew, as on macOS: `brew install cirope/tap/trk`. Or with your
distribution's package manager:

```sh
# Debian, Ubuntu and derivatives
curl -fLO https://github.com/cirope/trk-cli/releases/latest/download/trk_linux_amd64.deb
sudo apt install ./trk_linux_amd64.deb

# Fedora, RHEL, CentOS Stream, openSUSE
sudo dnf install https://github.com/cirope/trk-cli/releases/latest/download/trk_linux_amd64.rpm
# (openSUSE: sudo zypper install <same URL>)

# Alpine
curl -fLO https://github.com/cirope/trk-cli/releases/latest/download/trk_linux_amd64.apk
sudo apk add --allow-untrusted trk_linux_amd64.apk

# Any distribution (Arch, NixOS, a container...): the plain binary
curl -fL https://github.com/cirope/trk-cli/releases/latest/download/trk_linux_amd64.tar.gz | tar -xz trk
sudo install trk /usr/local/bin/
```

Packages install `trk` to `/usr/bin`. Upgrade by installing the newer
package the same way.

### Windows

Download `trk_windows_amd64.zip` from the
[latest release](https://github.com/cirope/trk-cli/releases/latest), unzip
it, and put `trk.exe` somewhere on your `PATH`.

### With Go

```sh
go install github.com/cirope/trk-cli/cmd/trk@latest
```

### Checking a download

Each release has a `checksums.txt` and a signed provenance attestation:

```sh
sha256sum --ignore-missing -c checksums.txt
gh attestation verify trk_linux_amd64.tar.gz --repo cirope/trk-cli
```

### Afterwards

Shell completions: `trk completion bash|zsh|fish|powershell --help`.

`trk` tells you, at most once a day, when a newer version exists (never
under `--json` or in a pipe; `TRK_NO_UPDATE_NOTIFIER=1` turns it off). It
never updates itself.

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
| | `TRK_NO_UPDATE_NOTIFIER=1` | Never say when a newer version exists |

## Development

Requires Go (the version is pinned in `go.mod`). One command runs every
check CI runs: formatting, `go mod tidy`, vet, staticcheck, golangci-lint,
the tests with the race detector, govulncheck, and builds for macOS, Linux
and Windows.

```sh
make check
make build   # bin/trk
```

Linters, GoReleaser and other tools are pinned in `tools/go.mod` and run
with `go tool`; nothing needs installing by hand. Tests never touch the
network, your keychain or your config: the end-to-end tests in
`cmd/trk/testdata/script` run trk against a fake Trk built from recorded
API answers (`internal/api/apitest/testdata/fixtures`). Golden files for
human output are updated with `go test ./internal/render -update`, and
reviewed like code.

### Releasing

1. Move the "Unreleased" notes in `CHANGELOG.md` under `## <version>`.
2. Tag and push: `git tag v<version> && git push origin v<version>`.

CI then runs every check and publishes the release, the packages, the
attestation and the Homebrew cask. `make release-snapshot` builds the
same locally without publishing. The repository needs a
`HOMEBREW_TAP_TOKEN` secret that can push to `cirope/homebrew-tap`.
