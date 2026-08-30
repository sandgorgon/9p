// Command 9pc is a bare 9P2000 client: ls, cat, get, and put against
// a running 9P server, for manual testing and quick scripting.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/client"
)

func main() {
	net := flag.String("net", "tcp", "dial network (e.g. tcp, unix)")
	addr := flag.String("addr", "localhost:5640", "server address (path when -net unix)")
	uname := flag.String("uname", "glenda", "attach user name")
	aname := flag.String("aname", "", "attach tree name")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
	}
	cmd, args := args[0], args[1:]

	c, err := client.Dial(*net, *addr)
	if err != nil {
		fatalf("dial: %v", err)
	}
	defer c.Close()
	root, err := c.Attach(*uname, *aname)
	if err != nil {
		fatalf("attach: %v", err)
	}

	switch cmd {
	case "ls":
		if len(args) != 1 {
			usage()
		}
		runLs(c, args[0])
	case "cat":
		if len(args) != 1 {
			usage()
		}
		runCat(c, args[0])
	case "get":
		if len(args) != 2 {
			usage()
		}
		runGet(c, args[0], args[1])
	case "put":
		if len(args) != 2 {
			usage()
		}
		runPut(c, root, args[0], args[1])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: 9pc [-net tcp|unix] [-addr host:port|path] [-uname name] [-aname tree] <command> ...

  9pc ls <path>
  9pc cat <path>
  9pc get <remote> <local>
  9pc put <local> <remote>
`)
	os.Exit(2)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "9pc: "+format+"\n", args...)
	os.Exit(1)
}

func splitPath(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func runLs(c *client.Client, remote string) {
	f, err := c.Open(remote, p9.OREAD)
	if err != nil {
		fatalf("open %s: %v", remote, err)
	}
	defer f.Close()
	entries, err := f.ReadDir()
	if err != nil {
		fatalf("readdir %s: %v", remote, err)
	}
	for _, e := range entries {
		kind := "-"
		if e.Qid.IsDir() {
			kind = "d"
		}
		fmt.Printf("%s %10d %s\n", kind, e.Length, e.Name)
	}
}

func runCat(c *client.Client, remote string) {
	f, err := c.Open(remote, p9.OREAD)
	if err != nil {
		fatalf("open %s: %v", remote, err)
	}
	defer f.Close()
	if _, err := io.Copy(os.Stdout, f); err != nil {
		fatalf("read %s: %v", remote, err)
	}
}

func runGet(c *client.Client, remote, local string) {
	in, err := c.Open(remote, p9.OREAD)
	if err != nil {
		fatalf("open %s: %v", remote, err)
	}
	defer in.Close()
	out, err := os.Create(local)
	if err != nil {
		fatalf("create %s: %v", local, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		fatalf("get %s: %v", remote, err)
	}
}

// runPut copies local's contents to remote, overwriting remote if it
// already exists. It tries Open first; only if that fails (remote
// doesn't exist yet) does it fall back to Walk-parent-and-Create.
func runPut(c *client.Client, root *client.Fid, local, remote string) {
	in, err := os.Open(local)
	if err != nil {
		fatalf("open %s: %v", local, err)
	}
	defer in.Close()

	out, err := c.Open(remote, p9.OWRITE)
	if err != nil {
		dir, name := path.Split(remote)
		if name == "" {
			fatalf("put: remote path %q has no file name", remote)
		}
		target, err := root.Walk(splitPath(dir)...)
		if err != nil {
			fatalf("walk %s: %v", dir, err)
		}
		_, _, err = target.Create(name, 0644, p9.OWRITE)
		target.Clunk()
		if err != nil {
			fatalf("create %s: %v", remote, err)
		}
		out, err = c.Open(remote, p9.OWRITE)
		if err != nil {
			fatalf("open %s: %v", remote, err)
		}
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		fatalf("put %s: %v", remote, err)
	}
}
