# Changelog

What changed in each release of `trk`, for the people who use it.

## Unreleased

The first release.

- `trk login` signs this machine in by approving in the browser; no
  password or token is typed. `trk logout` and `trk whoami`.
- `trk cards` lists your open cards, grouped as "My cards" is in Trk.
- `trk card show` shows a card; `--markdown` prints the same document as
  the web app's Download, to pipe into an agent; `--web` opens it.
- `trk card pull` writes a card and its files to `.trk/cards/<id>/`, and
  keeps them out of git.
- `trk card take` makes you an owner, turns Trk's agent off, moves the
  card to In progress, pulls it, and names the branch whose pull request
  finds its way back to the card. `--branch` creates it.
- `--json` on every command, and stable exit codes.
