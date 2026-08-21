# p9

[![CI](https://github.com/sandgorgon/9p/actions/workflows/ci.yml/badge.svg)](https://github.com/sandgorgon/9p/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/sandgorgon/9p)](https://github.com/sandgorgon/9p/releases/latest)
[![License: MIT](https://img.shields.io/github/license/sandgorgon/9p)](LICENSE)

[![Go](https://img.shields.io/badge/go-blue?logo=go&logoColor=white)](https://github.com/topics/go)
[![Plan 9](https://img.shields.io/badge/plan9-lightgrey)](https://github.com/topics/plan9)
[![9P2000](https://img.shields.io/badge/9p2000-lightgrey)](https://github.com/topics/9p2000)
[![Filesystem](https://img.shields.io/badge/filesystem-lightgrey)](https://github.com/topics/filesystem)
[![Network Protocol](https://img.shields.io/badge/network--protocol-lightgrey)](https://github.com/topics/network-protocol)

A pure-Go implementation of the Plan 9 filesystem protocol, **9P2000**
(the original 1992 spec — not the `.u`/`.L` Linux extensions), with
no dependencies beyond the standard library.

```
go get github.com/sandgorgon/9p
```

## Packages

| Package                | What it is |
|-------------------------|------------|
| `p9`                    | Wire encoding: message types, `Marshal`/`Unmarshal`, `Qid`, `Stat`, `Mode`. No I/O policy. |
| `p9/client`             | A 9P2000 client: dial or wrap a connection, `Attach`, `Walk`, `Open`, and a `File` implementing `io.Reader`/`Writer`/`ReaderAt`/`WriterAt`/`Seeker`/`Closer`. |
| `p9/server`             | A 9P2000 server: given a small `FileSystem`/`File` backend interface, handles wire encoding, fid bookkeeping, walk batching, and `Tflush` cancellation. |
| `p9/examples/memfs`     | An in-memory `server.FileSystem` — a demo backend and the server package's own test fixture. |
| `p9/examples/dirfs`     | A `server.FileSystem` that exports a real local directory tree, with every path validated to stay inside the configured root. |
| `p9/cmd/9ps`            | Serves a directory (or an empty in-memory filesystem with `-mem`) over TCP. |
| `p9/cmd/9pc`            | A bare CLI client: `ls`, `cat`, `get`, `put`. |

## Quick start

Serve a directory:

```go
fs, err := dirfs.New("/path/to/export")
srv := &server.Server{FS: fs}
l, _ := net.Listen("tcp", ":5640")
srv.Serve(l)
```

or from the command line:

```
go run ./cmd/9ps -addr :5640 -root /path/to/export
go run ./cmd/9pc -addr localhost:5640 ls /
go run ./cmd/9pc -addr localhost:5640 cat /some/file
go run ./cmd/9pc -addr localhost:5640 get /some/file ./local-copy
go run ./cmd/9pc -addr localhost:5640 put ./local-file /remote-name
```

Talk to a server as a client:

```go
c, err := client.Dial("tcp", "localhost:5640")
defer c.Close()

if _, err := c.Attach("glenda", ""); err != nil {
    log.Fatal(err)
}

f, err := c.Open("/some/file", p9.OREAD)
defer f.Close()
io.Copy(os.Stdout, f)
```

## Writing a server backend

A backend only implements two small interfaces — `server.FileSystem`
(one method, `Attach`) and `server.File` (walk to a child, open,
read/write at an offset, stat, remove). The `server` package handles
everything wire-related: message framing, fid allocation and
lifecycle, splitting long walks into ≤16-element `Twalk` batches, and
running each request in its own goroutine so a `Tflush` can cancel
one that's still in flight. See `examples/memfs` for the smallest
complete implementation, or `examples/dirfs` for one backed by real
files.

## Known limitations

- No authentication: `Tauth` always fails with "authentication not
  required," and the client always attaches with `NOFID`. Servers
  that require an auth handshake before attach aren't supported.
- Only the base 9P2000 spec — no `.u` (numeric uids, symlinks,
  special files) or `.L` (Linux-specific) extensions.
- `Tflush` cancellation reaches a backend's `context.Context`
  parameters, but `memfs`/`dirfs` don't check them mid-operation
  (their operations are fast enough that it wouldn't matter);
  backends fronting something slower can.
- `dirfs`'s `Qid.Path` is a hash of the absolute file path, not a
  real inode number, so it isn't stable across a rename performed
  outside the server.

## Testing

```
go build ./...
go vet ./...
go test ./...
go test -race ./...
go test -run=none -fuzz=FuzzUnmarshal -fuzztime=30s .
```

`FuzzUnmarshal` (in the root package) is the one place this matters
most: it's the function parsing untrusted bytes off the wire, and it
must never panic regardless of how malformed the input is.
