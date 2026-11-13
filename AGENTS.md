# Working in this repository

`trk` is the command-line client for trk (`../trk`). **`SPEC.md` is the
brief**: read it before writing anything. This file is the short list of
rules to follow while implementing it.

The repository starts with documents only. There is no Go code yet, and
Go may not be installed on the machine (`brew install go`).

## The product, in one paragraph

A developer or their coding agent takes a card from trk and works it in
their own environment. `trk login` signs in by browser approval;
`trk cards` and `trk card show` read; `trk card pull` writes the card and
its files to disk; `trk card take` sets the card up for outside work and
names the branch (`trk/card-<id>`) that makes the pull request find its
way back. Human-readable by default, `--json` for agents, stable exit
codes.

## Rules

### Process

- **Tests first.** Write the failing test, watch it fail for the right
  reason, then write the code. A test that passed the first time it ran
  proved nothing.
- **Run the whole gate before saying something is done**: format, vet,
  staticcheck, golangci-lint, `go test -race ./...`, govulncheck, and the
  cross-builds (SPEC 8.7). Report what ran and what it printed.
- **Try it for real.** Unit tests do not run the binary; `testscript`
  tests do, and the final check of any command is running the built
  binary against a local trk. Say plainly what was and was not tried.
- **Never commit or run git-modifying commands unless asked.** End each
  change with a suggested commit subject of 50 characters or less.
- **Ask before deciding the product.** SPEC section 10 lists open
  decisions with a recommendation each. Follow the recommendation to keep
  moving, flag that you did, and do not invent new product behaviour.

### Go

- The current stable Go; `go.mod` pins `go` and `toolchain`.
- Standard library first. A new dependency needs a line in SPEC 8.2
  saying why. No assertion libraries, no mocking frameworks, no HTTP or
  JSON or logging libraries.
- Development tools are pinned with the `tool` directive and run with
  `go tool`.
- Everything under `internal/`. `cmd/trk/main.go` only wires and exits.
- `context.Context` first on anything that does I/O, from
  `signal.NotifyContext` in `main`. Every request has a timeout.
- Wrap errors with what was being done; inspect with `errors.Is` /
  `errors.As`. One clear sentence for the user, the right exit code, no
  stack trace.
- No package-level state, no `init()` that does work, no `os.Exit`
  outside `main`. Dependencies (API client, writers, clock, file root,
  token store) are passed in.
- `gofmt`, `go vet`, `staticcheck` and `golangci-lint` clean. `go mod
  tidy` leaves no diff.

### Safety

- **A token never appears** in output, logs, errors, `--verbose`, test
  fixtures or commits. The token type redacts itself.
- **File names from the server are untrusted.** Everything `card pull`
  writes goes through an `os.Root` at the card's directory, to a
  temporary file renamed into place. Nothing is executed.
- Card content, and customer notes above all, are internal: `card pull`
  keeps `.trk/` out of git with `.git/info/exclude`.
- Tests never touch the developer's real config, keychain or network.

### Output

- Results on stdout, everything else on stderr.
- `--json` prints one JSON document and nothing else on stdout; its
  shape is a public contract (fields are added, never renamed or
  removed).
- No colour or spinners when stdout is not a terminal; honour `NO_COLOR`.
- Exit codes as in SPEC 4.8. They are tested.
- Every command's `--help` has an example.

### The server

- The API in SPEC section 7 **does not exist yet**. It is built in
  `../trk`, by that repository's rules (`../trk/AGENTS.md`). Do not work
  around a missing endpoint from the CLI: add it to the contract.
- The contract is kept honest by shared fixtures (SPEC 8.6). Change the
  contract by changing a fixture, in both repositories.
- Never point tests or experiments at `trk.hullop.com`. Use a local trk.

## Suggested order

1. Skeleton: module, `cmd/trk`, `--version`, `--help`, the gate, CI.
2. `internal/api`: the client, its errors, retries and timeouts, against
   `httptest` and the first fixtures.
3. `internal/auth`: the device flow with a fake clock; token storage.
4. `trk login`, `logout`, `whoami`, end to end with `testscript`.
5. `trk cards`, `trk card show` (human, `--json`, `--markdown`).
6. `internal/pull` and `trk card pull`, hostile names first.
7. `trk card take`.
8. Releases.

The server side (SPEC 7.4) can be built in parallel in `../trk`; steps 1
to 3 need nothing from it.
