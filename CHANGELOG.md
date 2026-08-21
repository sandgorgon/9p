# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project intends to follow [Semantic Versioning](https://semver.org/)
once a first tagged release is cut.

## [Unreleased]

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
