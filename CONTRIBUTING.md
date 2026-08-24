# Contributing

Thanks for considering a contribution to `p9`. This project has one
hard rule that shapes everything else: **no dependencies beyond the
Go standard library.** Please don't send a PR that adds one, even a
small or well-known one — the whole point of this implementation is
that `go build` never has to reach outside `GOROOT`.

## Getting started

```
git clone git@github.com:sandgorgon/9p.git
cd 9p
go build ./...
go test ./...
```

No other setup is required — no `go.sum`, no toolchain beyond `go`
itself.

## Branching (Gitflow)

This repo follows [Gitflow](https://nvie.com/posts/a-successful-git-branching-model/).
Two branches are permanent; everything else is a short-lived branch
merged in via PR and then deleted.

- **`master`** — release history only. Every commit on it is a
  tagged release (`vX.Y.Z`). Never commit to it directly; it only
  moves via a `release/*` or `hotfix/*` merge.
- **`develop`** — the integration branch, and GitHub's default
  branch. Unreleased work accumulates here, mirrored by the
  `[Unreleased]` section of `CHANGELOG.md`. Never commit to it
  directly either — merge in via PR from a `feature/*` branch.

Short-lived branch types:

| Branch      | Cut from  | Merges into        | Naming             | Purpose                                  |
|-------------|-----------|---------------------|--------------------|-------------------------------------------|
| `feature/*` | `develop` | `develop`           | `feature/<name>`   | New work, in progress or small changes    |
| `release/*` | `develop` | `master` + `develop`| `release/<X.Y.Z>`  | Stabilize a release (version bump, final CHANGELOG edits, no new features) |
| `fix/*`     | `develop` | `develop`           | `fix/<name>`       | Bug fix that isn't urgent enough for a hotfix |
| `hotfix/*`  | `master`  | `master` + `develop`| `hotfix/<name>`    | Urgent production fix, released outside the normal release cycle |

Everyday work (the common case):

```
git checkout develop
git pull
git checkout -b feature/my-change
# ... commit work ...
```

Open the PR against `develop`, not `master`. Once merged, delete the
branch — it's disposable.

Cutting a release:

```
git checkout -b release/X.Y.Z develop
# bump any version references, move CHANGELOG's [Unreleased]
# entries under a new [X.Y.Z] - YYYY-MM-DD heading
git checkout master && git merge --no-ff release/X.Y.Z
git tag vX.Y.Z
git checkout develop && git merge --no-ff release/X.Y.Z
git push origin master develop vX.Y.Z
git branch -d release/X.Y.Z
```

A `hotfix/*` branch follows the same shape but starts from `master`
instead of `develop`, for a fix that can't wait for the next regular
release.

## Before you send a PR

Run the same checks CI runs:

```
go build ./...
go vet ./...
gofmt -l .          # should print nothing
go test -race ./...
go test -run=^$ -fuzz=FuzzUnmarshal -fuzztime=30s .
```

A PR that fails any of these will fail CI, so it's worth running them
locally first.

## What to send

- **Bug fixes**: welcome, with a test that fails before the fix and
  passes after.
- **New `server.FileSystem`/`server.File` backends** (beyond
  `examples/memfs` and `examples/dirfs`): welcome as additions under
  `examples/`, as long as they stay stdlib-only.
- **Protocol extensions** (`.u`, `.L`, or similar): out of scope for
  this repo. This implementation deliberately targets the base
  9P2000 spec only; if you need an extension, please fork rather than
  extend the wire format here.
- **New third-party dependencies, build tools, or codegen steps**:
  won't be merged, regardless of how small or standard. If it's not
  something `go build` can resolve from the standard library alone,
  it doesn't belong here.

## Code conventions

- Match the existing style: `gofmt`-formatted, doc comments on every
  exported identifier, no comments that just restate what the code
  does.
- Keep the wire-protocol package (`p9`, the repo root) free of I/O
  policy — it only encodes and decodes messages. Connection handling
  belongs in `client` or `server`.
- If you touch `Unmarshal` or anything else that parses bytes off the
  wire, add cases to `message_test.go` and consider whether
  `FuzzUnmarshal`'s seed corpus in `fuzz_test.go` should grow too —
  that function exists specifically to guarantee malformed input
  never panics.
- Errors returned from a `server.File` method become the `Rerror`
  text sent to the client verbatim (`err.Error()`), so keep those
  messages short and free of anything sensitive (local paths,
  internal state).

## Reporting issues

Open a GitHub issue with what you expected, what happened instead,
and, if it's a protocol-level bug, the specific message sequence (or
a `p9.Message` round-trip) that reproduces it.
