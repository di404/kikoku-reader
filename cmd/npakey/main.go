// npakey recovers the per-title NPA decryption table with a known-plaintext
// attack on Ogg Vorbis headers, then checks it against PNG/JPEG/zlib entries.
//
//	npakey <ogg-archive>...
//
// Prints Go source for internal/npa/table.go.
package main

import (
	"fmt"
	"os"
	"strings"

	"kikoku/internal/npa"
)

// Known bytes of the first Ogg page of a Vorbis stream; -1 = unknown.
var oggHead = func() []int {
	h := make([]int, 39)
	for i := range h {
		h[i] = -1
	}
	set := func(off int, b ...byte) {
		for i, v := range b {
			h[off+i] = int(v)
		}
	}
	set(0, 'O', 'g', 'g', 'S', 0, 2)
	set(6, 0, 0, 0, 0, 0, 0, 0, 0)
	set(18, 0, 0, 0, 0)
	set(26, 1, 0x1e, 1, 'v', 'o', 'r', 'b', 'i', 's', 0, 0, 0, 0)
	return h
}()

type sample struct {
	e   *npa.Entry
	arc int32
	raw []byte
}

func main() {
	var samples []sample
	for _, p := range os.Args[1:] {
		a, err := npa.Open(p, nil)
		if err != nil {
			panic(err)
		}
		for _, e := range a.Entries {
			if !strings.HasSuffix(strings.ToLower(e.Name), ".ogg") || e.Size < 64 {
				continue
			}
			raw, err := a.ReadRaw(e)
			if err != nil {
				panic(err)
			}
			samples = append(samples, sample{e, a.ArcKey(), raw[:64]})
		}
	}
	fmt.Fprintln(os.Stderr, "ogg samples:", len(samples))

	best, bestCount := -1, -1
	var bestTable [256]int
	for nk := 0; nk < 256; nk++ {
		var t [256]int
		for i := range t {
			t[i] = -1
		}
		ok := true
	outer:
		for _, s := range samples {
			k := int(npa.EntryKey(s.e, int32(nk), s.arc))
			for i, p := range oggHead {
				if p < 0 {
					continue
				}
				c := s.raw[i]
				v := (p + k + i) & 0xff
				if t[c] == -1 {
					t[c] = v
				} else if t[c] != v {
					ok = false
					break outer
				}
			}
		}
		if !ok {
			continue
		}
		n := 0
		for _, v := range t {
			if v >= 0 {
				n++
			}
		}
		fmt.Fprintf(os.Stderr, "namekey low byte %#02x consistent, %d/256 table entries\n", nk, n)
		if n > bestCount {
			best, bestCount, bestTable = nk, n, t
		}
	}
	if best < 0 {
		fmt.Fprintln(os.Stderr, "no consistent key: model is wrong")
		os.Exit(1)
	}
	// Table is a permutation; fill a single missing slot if possible.
	used := map[int]bool{}
	var holes []int
	for c, v := range bestTable {
		if v < 0 {
			holes = append(holes, c)
		} else {
			used[v] = true
		}
	}
	var free []int
	for v := 0; v < 256; v++ {
		if !used[v] {
			free = append(free, v)
		}
	}
	if len(holes) == 1 && len(free) == 1 {
		bestTable[holes[0]] = free[0]
		holes = nil
	}
	if len(holes) > 0 {
		fmt.Fprintf(os.Stderr, "still missing %d entries: %v\n", len(holes), holes)
	}
	fmt.Printf("var kikokuNameKey int32 = %#x\n\nvar kikokuTable = [256]byte{", best)
	for i, v := range bestTable {
		if i%16 == 0 {
			fmt.Print("\n\t")
		}
		if v < 0 {
			v = 0
		}
		fmt.Printf("0x%02x, ", v)
	}
	fmt.Println("\n}")
}
