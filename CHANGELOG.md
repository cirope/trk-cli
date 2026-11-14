# Changelog

What changed in each release of `trk`, for the people who use it.

## 0.3.0

- `trk boards` lists boards as a tree, subprojects under their parent.
- `trk board new <name>` creates a board or a subproject (`--parent`),
  with `--repo`, `--language`, `--status` and `--detail Name=Value`.
- `trk board edit <board>` changes a board; `--detail` adds or replaces
  one detail and `--remove-detail` removes one, keeping the rest.
- `trk card own <card> [person…]` and `trk card unown` add and remove a
  card's owners: you by default, or people by email or id.
- `trk card say` says whether Trk's agent took a turn (the card back in
  In progress) or the message was a note (the card stays put).
- `trk card new -q --take` prints only the new card's id, so
  `id=$(trk card new -q --take …)` works.

## 0.2.1

- `trk card move <card> <column>` moves a card, by column name or role.
  Moving into done closes the card and merges its pull request, so trk
  asks first, or needs `--yes`. `trk card done <card>` is the shortcut.
- `trk card say <card> "text"` (or `trk card reply`) posts on a card's
  conversation; `--file -` reads stdin. trk says when Trk's agent will
  answer.

## 0.2.0

- `trk card new --board <board> "Title"` creates a card, with
  `--description` (or `--description-file -` for stdin), `--type`,
  `--priority`, `--due`, `--column`, and `--take` to take it right away.
  It prints the new card's id and URL; `-q` prints only the id.
- When Trk rejects a field, trk says which one, and exits with 2.
- `trk card show` lists the board's details (Docs, Website, Tests…) that
  the team set for whoever works the card; `--json` has them as
  `board_details`.

## 0.1.1

- `brew install cirope/tap/trk` works: the Homebrew cask in 0.1.0 could
  not be read.

## 0.1.0

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
