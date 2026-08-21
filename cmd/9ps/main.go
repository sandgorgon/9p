// Command 9ps serves a filesystem over 9P2000 on a TCP address:
// either a real directory (the default) or an in-memory scratch
// filesystem (-mem).
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/sandgorgon/9p/examples/dirfs"
	"github.com/sandgorgon/9p/examples/memfs"
	"github.com/sandgorgon/9p/server"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: 9ps [-addr host:port] [-root dir | -mem]

Serves a filesystem over 9P2000 on a TCP address: either a real
directory (-root, the default) or an empty in-memory scratch
filesystem (-mem).

Flags:
`)
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	addr := flag.String("addr", ":5640", "address to listen on")
	root := flag.String("root", ".", "directory to export (ignored with -mem)")
	mem := flag.Bool("mem", false, "serve an empty in-memory filesystem instead of -root")
	flag.Parse()

	var fs server.FileSystem
	if *mem {
		fs = memfs.New()
	} else {
		d, err := dirfs.New(*root)
		if err != nil {
			log.Fatalf("9ps: %v", err)
		}
		fs = d
	}

	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("9ps: %v", err)
	}
	fmt.Printf("9ps: serving on %s\n", l.Addr())

	srv := &server.Server{FS: fs}
	log.Fatal(srv.Serve(l))
}
