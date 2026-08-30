# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project intends to follow [Semantic Versioning](https://semver.org/)
once a first tagged release is cut.

## [Unreleased]

### Fixed

- `cmd/9pc`: `put` always went through Walk-parent + `Create` before
  opening the remote file for write, but `Tcreate` on an existing
  name is a protocol error, so `put` aborted on any remote path that
  already existed (e.g. a server exposing a file meant to be edited
  in place, not created). `put` now tries `Open` first and only
  falls back to Walk+Create when the file doesn't exist yet.

## [0.7.0] - 2026-08-30

### Added

- `client.Fid.OpenFile`/`OpenFileContext` and `Fid.CreateFile`/
  `CreateFileContext`, returning an I/O-capable `*File` positioned at
  the fid instead of just a `Qid`. A caller that already holds a
  walked `*Fid` (cheap, no I/O) can now get I/O on it directly,
  instead of discarding the fid and re-walking by path string via
  `Client.Open`/`Client.Create` just to obtain a `*File` — removing
  one `Twalk` round-trip per open. `Client.Open`/`Client.Create` are
  now implemented in terms of the new methods.

## [0.6.0] - 2026-08-29

### Added

- `cmd/9pc`: a `-net` flag (default `tcp`), passed through to
  `client.Dial` alongside `-addr`. `client.Dial`'s `network` parameter
  was already a plain `net.Dial` passthrough, so `-net unix` with
  `-addr` set to a socket path works today with no `client` changes —
  the CLI just never exposed the option. Lets `9pc` script against a
  local 9P server over a Unix domain socket instead of requiring a
  TCP port.

## [0.5.0] - 2026-08-24

### Added

- `server.Server.MaxConcurrentRequests`: caps how many requests from
  one connection are dispatched into `FS` at once. 9P2000 lets a
  client pipeline any number of tagged requests on a single
  connection, and until now `Server` spawned an unconditional
  goroutine into `FS` for every one it read, with no way for a caller
  to bound that. Zero (the default) keeps today's unlimited behavior.
  `Tflush` is always exempt from the limit, so a client can still
  cancel a request holding a slot even at the concurrency cap.

### Changed

- Bumped the fuzz smoke test's `-fuzztime` from 20s to 30s (CI and
  `CONTRIBUTING.md` both), after it flaked once on a `master` CI run
  with `context deadline exceeded` right at the 20s boundary — no
  crasher found, just the fuzz engine's own deadline racing against
  `-fuzztime`'s cutoff on a loaded runner.

## [0.4.0] - 2026-08-23

### Fixed

- `server`: `Tclunk` and `Tremove` discarded whatever error
  `File.Close` returned, always replying success regardless — despite
  `File`'s own doc comment promising every method's error reaches the
  client as an `Rerror`. `Tclunk` now reports a `Close` error as an
  `Rerror`; `Tremove` does the same when `Remove` itself succeeded but
  the `Close` that follows it doesn't. The fid is released either way
  in both cases, matching normal 9P2000 clunk semantics.

## [0.3.0] - 2026-08-23

### Added

- `client.Client.Create`/`CreateContext`: the write-side counterpart
  to `Client.Open` — walks to path's parent, creates its final
  element there, and returns a `*File` open for I/O on it. Previously
  `Fid.Create` existed at the wire level but nothing in the public API
  could read or write the file it created.

### Changed

- Bumped `go.mod`'s `go` directive from 1.22 to 1.26.

## [0.2.1] - 2026-08-23

### Fixed

- `client.File.ReadDir`/`ReadDirContext`: silently returned an
  incomplete listing, with no error, for any directory whose encoded
  entries didn't fit in one negotiated-`msize` read. `ReadDirContext`
  went through the same `readAt` helper used for regular file reads,
  which treats a reply shorter than requested as end-of-file — correct
  for `io.ReaderAt`, but wrong for directory reads: `server.MarshalDir`
  (added in 0.2.0) only ever returns whole `Stat` entries, so it
  returns fewer bytes than requested on essentially every read short
  of the last. `ReadDirContext` now issues its own `Tread` calls and
  stops only on a truly empty reply, matching the 9P2000 directory-read
  convention.

## [0.2.0] - 2026-08-23

### Added

- `server.Server.ConnContext`: an optional hook, called once per
  accepted connection before its requests are served, to build that
  connection's base `context.Context` — for per-connection data such
  as a TLS client identity to reach `FileSystem.Attach` and every
  `File` method. Mirrors `net/http.Server.ConnContext`.
- `server.Server.ServeConnContext`: `ServeConn` with an explicit base
  context, for callers that build their own per-connection context by
  hand instead of going through `Serve`/`ConnContext`.
- `server.MarshalDir`: encodes a `[]p9.Stat` and a client-requested
  offset/buffer into a directory `Read` reply, handling the
  whole-entries-only, never-split-a-Stat-blob contract 9P2000
  directory reads require so a `server.File` implementation doesn't
  have to reimplement that bookkeeping itself.

### Fixed

- `examples/memfs` and `examples/dirfs`: directory `Read` could split
  a `Stat` blob across a read boundary when a directory's listing
  exceeded the client's requested buffer size, corrupting the reply
  (`client.File.ReadDir` would fail with "truncated entry"). Both now
  use `server.MarshalDir`, which returns whole entries only.

## [0.1.0] - 2026-08-20

Initial release.

### Added

- `p9`: 9P2000 wire encoding — message types, `Marshal`/`Unmarshal`,
  `Qid`, `Stat`, `Mode`, with round-trip tests and a `FuzzUnmarshal`
  target guarding against panics on malformed input.
- `p9/client`: a 9P2000 client — dial or wrap a connection, `Attach`,
  batched `Walk`, `Open`/`Create`/`Stat`/`WStat`/`Remove`, and a
  `File` implementing `io.Reader`/`Writer`/`ReaderAt`/`WriterAt`/
  `Seeker`/`Closer`.
- `p9/server`: a 9P2000 server built on a small `FileSystem`/`File`
  backend interface — handles fid bookkeeping, multi-element walk
  batching, and per-request `Tflush` cancellation.
- `p9/examples/memfs`: an in-memory `server.FileSystem` backend, used
  as both a demo and the server package's test fixture.
- `p9/examples/dirfs`: a `server.FileSystem` backend exporting a real
  local directory, with path-traversal protection.
- `p9/cmd/9ps`: a CLI to serve a directory (or an in-memory
  filesystem) over TCP.
- `p9/cmd/9pc`: a bare CLI client (`ls`, `cat`, `get`, `put`).
- `README.md`, `CONTRIBUTING.md`, and an MIT `LICENSE`.
- GitHub Actions CI: build, vet, `gofmt` check, `go test -race`, and
  a `FuzzUnmarshal` smoke run on every push and pull request.

### Changed

- Module path set to `github.com/sandgorgon/9p` (was the bare `p9`),
  so `go get github.com/sandgorgon/9p` works.

[Unreleased]: https://github.com/sandgorgon/9p/compare/v0.1.0...master
[0.1.0]: https://github.com/sandgorgon/9p/releases/tag/v0.1.0
