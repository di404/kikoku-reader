package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"kikoku/internal/assets"
)

func TestStreamDecodesSound(t *testing.T) {
	home, _ := os.UserHomeDir()
	store, err := assets.Open(filepath.Join(home, "Games", "kikoku-remake"))
	if err != nil {
		t.Skip(err)
	}
	data, err := store.ReadFile("voice/01/0200010a01.ogg")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newStream(data, false)
	if err != nil {
		t.Fatal(err)
	}
	s.gain, s.target = 1, 1
	buf := make([]byte, 8*rate)
	total, peak := 0, 0.0
	for {
		n, err := s.Read(buf)
		for i := 0; i+4 <= n; i += 4 {
			peak = math.Max(peak, math.Abs(float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[i:])))))
		}
		total += n
		if err != nil {
			break
		}
	}
	secs := float64(total) / 8 / rate
	t.Logf("decoded %.2fs, peak %.3f", secs, peak)
	if secs < 0.3 || peak < 0.05 {
		t.Fatalf("looks silent: %.2fs peak %.3f", secs, peak)
	}
}
