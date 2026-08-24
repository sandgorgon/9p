# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project intends to follow [Semantic Versioning](https://semver.org/)
once a first tagged release is cut.

## [Unreleased]

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
