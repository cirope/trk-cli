# Changelog

What changed in each release of `trk`, for the people who use it.

## 0.4.7

- `trk inbox --snoozed` and `--done` say which list is empty.
- `trk card say --mention` warns when Trk called fewer people than named
  (never you, only members).

## 0.4.6

- `trk inbox`, `trk inbox done` and `trk inbox snooze`: the cards waiting
  for you, and why.
- `trk card say --mention <person>` calls people to a card with a note.
- Areas: `trk board new|edit --area <name> --covers … --people …`,
  `--remove-area`; `trk card new|edit --area`; `card show` shows the area.
- `trk card show` names who wrote each message.

## 0.4.5

- `trk card link <card> <commit>` links a commit (a sha, a commit URL or
  a git ref such as `HEAD`); `trk card show` lists a card's commits.

## 0.4.4

- `trk card attach <card> <file>…` attaches files; `trk card say --attach`
  sends them with a message.
- `trk mail` lists email Trk held; `trk mail file` and `trk mail discard`
  (`--block`) act on it.
- `trk card show` says when a card came by email and names the sender of
  customer messages.

## 0.4.3

- `trk card rate <card> good|bad` rates Trk's agent's latest answer,
  with `--note` and `--sent`.

## 0.4.2

- When Trk refuses a value, trk no longer suggests `trk --help`.

## 0.4.1

- `trk card link <card> <pull request url>` links a pull request
  opened from another branch.

## 0.4.0

- `trk card edit <card>` changes a card's title, description, type,
  priority or due date; only what you give changes.
- `trk card show` shows the response Trk's agent proposes for the
  customer on a question, with its note under it; `--json` has it as
  `messages[].proposed_response`.

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
