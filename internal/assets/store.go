// Package assets maps the script's resource paths ("cg/ev/x.jpg",
// "voice/01/xxxx", "sound/bgm/m06") onto entries in the game's NPA archives.
package assets

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"kikoku/internal/npa"
)

type Store struct {
	Root  string
	cg    []*npa.Archive // patch first
	voice *npa.Archive
	sound *npa.Archive
	nss   []*npa.Archive // patch first

	mu   sync.Mutex
	dims map[string][2]int
}

func Open(root string) (*Store, error) {
	s := &Store{Root: root, dims: map[string][2]int{}}
	scheme := npa.Kikokugai()
	open := func(name string, required bool) (*npa.Archive, error) {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err != nil {
			if required {
				return nil, fmt.Errorf("缺少 %s：%w", name, err)
			}
			return nil, nil
		}
		return npa.Open(p, scheme)
	}
	var err error
	for _, n := range []string{"cg-1.npa", "cg.npa"} {
		a, e := open(n, n == "cg.npa")
		if e != nil {
			return nil, e
		}
		if a != nil {
			s.cg = append(s.cg, a)
		}
	}
	for _, n := range []string{"nss-1.npa", "nss.npa"} {
		a, e := open(n, n == "nss.npa")
		if e != nil {
			return nil, e
		}
		if a != nil {
			s.nss = append(s.nss, a)
		}
	}
	if s.voice, err = open("voice.npa", true); err != nil {
		return nil, err
	}
	if s.sound, err = open("sound.npa", true); err != nil {
		return nil, err
	}
	return s, nil
}

// Find resolves a script path to an archive entry.
func (s *Store) Find(p string) (*npa.Archive, *npa.Entry) {
	p = strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "/")
	head, rest, _ := strings.Cut(p, "/")
	var arcs []*npa.Archive
	switch strings.ToLower(head) {
	case "cg", "cg2":
		arcs = s.cg
	case "voice":
		arcs = []*npa.Archive{s.voice}
	case "sound":
		arcs = []*npa.Archive{s.sound}
	case "nss":
		arcs = s.nss
	default:
		return nil, nil
	}
	cands := []string{rest}
	if path.Ext(rest) == "" {
		cands = append(cands, rest+".ogg", rest+".png", rest+".jpg")
	}
	for _, a := range arcs {
		for _, c := range cands {
			if e := a.Lookup(c); e != nil {
				return a, e
			}
		}
	}
	return nil, nil
}

func (s *Store) Exists(p string) bool { _, e := s.Find(p); return e != nil }

func (s *Store) ReadFile(p string) ([]byte, error) {
	a, e := s.Find(p)
	if e == nil {
		return nil, fmt.Errorf("资源不存在：%s", p)
	}
	return a.Read(e)
}

// Script returns the content of a .nss file, preferring the Chinese patch.
func (s *Store) Script(name string) ([]byte, error) { return s.ReadFile("nss/" + name) }

// Canonical returns the path with the extension the archive actually has.
func (s *Store) Canonical(p string) string {
	_, e := s.Find(p)
	if e == nil {
		return p
	}
	if path.Ext(p) == "" {
		return p + path.Ext(strings.ToLower(e.Name))
	}
	return p
}

// HiRes returns the 2048-wide "_l" variant of a cg image when the archive
// has one, otherwise the path itself. "cg/ev/ev_j01.jpg" -> "cg/ev/l/ev_j01_l.jpg".
func (s *Store) HiRes(p string) string {
	dir, file := path.Split(p)
	ext := path.Ext(file)
	base := strings.TrimSuffix(file, ext)
	for _, suf := range []string{"_m", "_l"} {
		if strings.HasSuffix(base, suf) && strings.HasSuffix(strings.TrimSuffix(dir, "/"), suf[1:]) {
			// already a scaled variant: cg/ef/m/x_m.jpg -> try cg/ef/l/x_l.jpg
			dir = path.Dir(strings.TrimSuffix(dir, "/")) + "/"
			base = strings.TrimSuffix(base, suf)
			break
		}
	}
	hi := dir + "l/" + base + "_l" + ext
	if s.Exists(hi) {
		return hi
	}
	return p
}

// ImageSize returns the pixel size of an image, reading only its header.
func (s *Store) ImageSize(p string) (int, int, error) {
	s.mu.Lock()
	d, ok := s.dims[p]
	s.mu.Unlock()
	if ok {
		return d[0], d[1], nil
	}
	a, e := s.Find(p)
	if e == nil {
		return 0, 0, errors.New("not found")
	}
	var cfg image.Config
	var err error
	for _, n := range []int{64 << 10, int(e.Size)} {
		var b []byte
		if b, err = a.ReadPrefix(e, n); err != nil {
			return 0, 0, err
		}
		if cfg, _, err = image.DecodeConfig(bytes.NewReader(b)); err == nil {
			break
		}
	}
	if err != nil {
		return 0, 0, err
	}
	s.mu.Lock()
	s.dims[p] = [2]int{cfg.Width, cfg.Height}
	s.mu.Unlock()
	return cfg.Width, cfg.Height, nil
}

// MoviePath maps "dx/xxx.ngs" to the file on disk.
func (s *Store) MoviePath(p string) string {
	return filepath.Join(s.Root, filepath.FromSlash(p))
}
