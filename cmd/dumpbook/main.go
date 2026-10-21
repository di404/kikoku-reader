// dumpbook compiles the story and prints steps, for checking the compiler.
//
//	dumpbook [-game DIR] [-from N] [-n N] [-json]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"kikoku/internal/assets"
	"kikoku/internal/script"
)

func main() {
	home, _ := os.UserHomeDir()
	game := flag.String("game", filepath.Join(home, "Games", "kikoku-remake"), "game folder")
	from := flag.Int("from", 0, "first step")
	n := flag.Int("n", 20, "steps to print")
	asJSON := flag.Bool("json", false, "print JSON")
	flag.Parse()

	store, err := assets.Open(*game)
	check(err)
	files, err := script.Chain(store, "0101h.nss")
	check(err)
	book, err := script.Compile(store, files)
	check(err)
	fmt.Fprintf(os.Stderr, "scenes=%d steps=%d warnings=%d\n", len(book.Scenes), len(book.Steps), len(book.Warnings))
	for _, w := range book.Warnings {
		fmt.Fprintln(os.Stderr, "  ", w)
	}
	for i := *from; i < *from+*n && i < len(book.Steps); i++ {
		s := book.Steps[i]
		if *asJSON {
			b, _ := json.Marshal(s)
			fmt.Println(string(b))
			continue
		}
		fmt.Printf("#%d %s %s app=%v voice=%s spk=%s\n  %q\n", i, s.Scene, s.Label, s.Append, s.Voice, s.Speaker, s.HTML)
		for _, l := range s.Layers {
			fmt.Printf("    [%s z=%d %.0f,%.0f %.0fx%.0f op=%.2f %s%s]\n", l.Kind, l.Z, l.X, l.Y, l.W, l.H, l.Opacity, l.Src, l.Color)
		}
		for _, l := range s.Loops {
			fmt.Printf("    ♪ %s %.2f\n", l.Src, l.Vol)
		}
		for _, l := range s.SE {
			fmt.Printf("    SE %s\n", l.Src)
		}
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
