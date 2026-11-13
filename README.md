# trk CLI

A command-line client for [trk](../trk), Hullop's support board: take a
card to your own terminal and your own agent, and let the pull request
find its way back.

```
trk login
trk card take BaHtsbhzVDa9 --branch
claude "Read .trk/cards/BaHtsbhzVDa9/card.md and its attachments, then do what the card asks."
git push -u origin HEAD && gh pr create --fill
```

## Status

**Not built yet.** This repository holds the brief:

- [`SPEC.md`](SPEC.md): what we want, in full: the commands, how signing
  in works, the server API it needs, and how it must be built and tested.
- [`AGENTS.md`](AGENTS.md): the rules for whoever implements it.

The server half (an API and browser approval of sessions) does not exist
in trk yet either; `SPEC.md` section 7 is its contract.

## In short

- Written in Go, one binary named `trk`.
- Signs in by approving in the browser. No secret is typed or pasted.
- `trk cards`, `trk card show`, `trk card pull`, `trk card take`.
- Readable for people, `--json` for agents, stable exit codes.
- The standard library first, a handful of justified dependencies, tests
  written first, and a single quality gate that CI runs on macOS, Linux
  and Windows.
