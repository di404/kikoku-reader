// Package npa reads Nitroplus NPA archives (NPA\x01) as used by the 2011
// remake of Kikokugai. Format follows GARbro's ArcNPA; the per-title
// decryption table is recovered separately (see table.go).
package npa

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/text/encoding/japanese"
)

const headerSize = 41

type Entry struct {
	Name         string // decoded from cp932, '\' separators
	RawName      []byte // decrypted cp932 bytes, used for the content key
	Offset       int64  // absolute in file
	Size         uint32
	UnpackedSize uint32
}

type Archive struct {
	f          *os.File
	Path       string
	Key1, Key2 int32
	Compressed bool
	Encrypted  bool
	Entries    []*Entry
	byName     map[string]*Entry
	Scheme     *Scheme
}

// Scheme holds what is specific to one title: the name key seed and the
// 256-byte decryption table.
type Scheme struct {
	NameKey int32
	Table   [256]byte
}

func Open(path string, scheme *Scheme) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a, err := parse(f, path)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	a.Scheme = scheme
	return a, nil
}

func (a *Archive) Close() error { return a.f.Close() }

func parse(f *os.File, path string) (*Archive, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil, err
	}
	if string(hdr[:4]) != "NPA\x01" {
		return nil, errors.New("not an NPA archive")
	}
	le := binary.LittleEndian
	a := &Archive{
		f:          f,
		Path:       path,
		Key1:       int32(le.Uint32(hdr[7:])),
		Key2:       int32(le.Uint32(hdr[11:])),
		Compressed: hdr[15] != 0,
		Encrypted:  hdr[16] != 0,
		byName:     map[string]*Entry{},
	}
	total := int(le.Uint32(hdr[17:]))
	dirSize := le.Uint32(hdr[37:])
	index := make([]byte, dirSize)
	if _, err := io.ReadFull(f, index); err != nil {
		return nil, err
	}
	key := a.ArcKey()
	dec := japanese.ShiftJIS.NewDecoder()
	p := 0
	for i := 0; i < total; i++ {
		if p+4 > len(index) {
			return nil, errors.New("truncated index")
		}
		n := int(le.Uint32(index[p:]))
		if n < 0 || p+4+n+17 > len(index) {
			return nil, errors.New("bad name size")
		}
		raw := append([]byte(nil), index[p+4:p+4+n]...)
		info := index[p+4+n:]
		p += 4 + n + 17
		if info[0] == 1 { // directory
			continue
		}
		for x := range raw {
			raw[x] += decryptName(x, i, key)
		}
		name, err := dec.Bytes(raw)
		if err != nil {
			name = raw
		}
		e := &Entry{
			Name:         string(name),
			RawName:      raw,
			Offset:       int64(dirSize) + int64(le.Uint32(info[5:])) + headerSize,
			Size:         le.Uint32(info[9:]),
			UnpackedSize: le.Uint32(info[13:]),
		}
		a.Entries = append(a.Entries, e)
		a.byName[normName(e.Name)] = e
	}
	return a, nil
}

func normName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "/", "\\"))
}

func (a *Archive) ArcKey() int32 { return a.Key1 * a.Key2 }

func decryptName(index, curfile int, arcKey int32) byte {
	k := int32(0xFC * index)
	k -= arcKey >> 24
	k -= arcKey >> 16
	k -= arcKey >> 8
	k -= arcKey & 0xff
	c := int32(curfile)
	k -= c >> 24
	k -= c >> 16
	k -= c >> 8
	k -= c
	return byte(k)
}

// Lookup finds an entry by path, case-insensitive, either separator.
func (a *Archive) Lookup(name string) *Entry { return a.byName[normName(name)] }

// EntryKey is the per-entry byte subtracted during decryption.
func EntryKey(e *Entry, nameKey, arcKey int32) byte {
	k := nameKey
	for _, b := range e.RawName {
		k -= int32(b)
	}
	k *= int32(len(e.RawName))
	k += arcKey
	k *= int32(e.UnpackedSize)
	return byte(k)
}

func EncryptedLen(e *Entry) int {
	n := 0x1000 + len(e.RawName)
	if n > int(e.Size) {
		n = int(e.Size)
	}
	return n
}

// ReadRaw returns the stored bytes of an entry without decrypting.
func (a *Archive) ReadRaw(e *Entry) ([]byte, error) {
	buf := make([]byte, e.Size)
	_, err := a.f.ReadAt(buf, e.Offset)
	return buf, err
}

func isImage(name string) bool {
	n := strings.ToLower(name)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".bmp", ".tga", ".gif"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}

// ReadPrefix returns up to n decrypted bytes from the start of an
// uncompressed entry; enough to read image headers without the whole file.
func (a *Archive) ReadPrefix(e *Entry, n int) ([]byte, error) {
	if a.Compressed && !isImage(e.Name) {
		return a.Read(e)
	}
	if n > int(e.Size) {
		n = int(e.Size)
	}
	buf := make([]byte, n)
	if _, err := a.f.ReadAt(buf, e.Offset); err != nil {
		return nil, err
	}
	return a.decrypt(e, buf)
}

func (a *Archive) decrypt(e *Entry, buf []byte) ([]byte, error) {
	if !a.Encrypted {
		return buf, nil
	}
	if a.Scheme == nil {
		return nil, errors.New("archive is encrypted but no scheme given")
	}
	k := EntryKey(e, a.Scheme.NameKey, a.ArcKey())
	n := EncryptedLen(e)
	if n > len(buf) {
		n = len(buf)
	}
	for i := 0; i < n; i++ {
		buf[i] = a.Scheme.Table[buf[i]] - k - byte(i)
	}
	return buf, nil
}

// Read returns the decrypted and, if needed, decompressed content.
func (a *Archive) Read(e *Entry) ([]byte, error) {
	buf, err := a.ReadRaw(e)
	if err != nil {
		return nil, err
	}
	if buf, err = a.decrypt(e, buf); err != nil {
		return nil, err
	}
	if a.Compressed && !isImage(e.Name) {
		zr, err := zlib.NewReader(bytes.NewReader(buf))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name, err)
		}
		defer zr.Close()
		out := make([]byte, 0, e.UnpackedSize)
		w := bytes.NewBuffer(out)
		if _, err := io.Copy(w, zr); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name, err)
		}
		return w.Bytes(), nil
	}
	return buf, nil
}
