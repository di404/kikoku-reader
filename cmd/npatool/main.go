// npatool lists, inspects and extracts NPA archives.
//
//	npatool list  <archive>
//	npatool raw   <archive> <entry>        hexdump first stored bytes
//	npatool x     <archive> <outdir> [prefix]
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kikoku/internal/npa"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: npatool list|raw|x <archive> ...")
		os.Exit(2)
	}
	a, err := npa.Open(os.Args[2], npa.Kikokugai())
	check(err)
	defer a.Close()
	switch os.Args[1] {
	case "list":
		fmt.Printf("key1=%#x key2=%#x compressed=%v encrypted=%v entries=%d\n",
			uint32(a.Key1), uint32(a.Key2), a.Compressed, a.Encrypted, len(a.Entries))
		for _, e := range a.Entries {
			fmt.Printf("%10d %10d  %s\n", e.Size, e.UnpackedSize, e.Name)
		}
	case "raw":
		e := a.Lookup(os.Args[3])
		if e == nil {
			check(fmt.Errorf("no entry %q", os.Args[3]))
		}
		b, err := a.ReadRaw(e)
		check(err)
		if len(b) > 256 {
			b = b[:256]
		}
		fmt.Print(hex.Dump(b))
	case "x":
		out := os.Args[3]
		prefix := ""
		if len(os.Args) > 4 {
			prefix = strings.ToLower(os.Args[4])
		}
		n := 0
		for _, e := range a.Entries {
			if !strings.HasPrefix(strings.ToLower(e.Name), prefix) {
				continue
			}
			b, err := a.Read(e)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			p := filepath.Join(out, filepath.FromSlash(strings.ReplaceAll(e.Name, "\\", "/")))
			check(os.MkdirAll(filepath.Dir(p), 0o755))
			check(os.WriteFile(p, b, 0o644))
			n++
		}
		fmt.Println("extracted", n)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
