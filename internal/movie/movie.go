// Package movie transcodes the remake's .ngs cutscenes (MPEG-1 program
// streams) to H.264 MP4 with ffmpeg so the web view can play them.
package movie

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type Cache struct {
	dir     string
	resolve func(scriptPath string) string

	mu      sync.Mutex
	pending map[string]*sync.Once
	errs    map[string]error
}

func NewCache(dir string, resolve func(string) string) *Cache {
	return &Cache{dir: dir, resolve: resolve, pending: map[string]*sync.Once{}, errs: map[string]error{}}
}

// diskName finds the file for a script path. The archive this copy came
// from stored Shift-JIS names that were extracted as GBK, so
// "mv0004煙01.ngs" is on disk as "mv0004墝01.ngs".
func (c *Cache) diskName(src string) (string, error) {
	p := c.resolve(src)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	dir, base := filepath.Split(p)
	if sj, err := japanese.ShiftJIS.NewEncoder().String(base); err == nil {
		if gb, err := simplifiedchinese.GBK.NewDecoder().String(sj); err == nil {
			if _, err := os.Stat(filepath.Join(dir, gb)); err == nil {
				return filepath.Join(dir, gb), nil
			}
		}
	}
	return "", fmt.Errorf("找不到过场 %s", src)
}

// Get returns the path of the transcoded MP4, converting on first use.
func (c *Cache) Get(src string) (string, error) {
	in, err := c.diskName(src)
	if err != nil {
		return "", err
	}
	h := sha1.Sum([]byte(src))
	out := filepath.Join(c.dir, hex.EncodeToString(h[:8])+".mp4")
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	c.mu.Lock()
	once := c.pending[out]
	if once == nil {
		once = &sync.Once{}
		c.pending[out] = once
	}
	c.mu.Unlock()
	once.Do(func() {
		err := c.transcode(in, out)
		c.mu.Lock()
		c.errs[out] = err
		c.mu.Unlock()
	})
	c.mu.Lock()
	err = c.errs[out]
	c.mu.Unlock()
	return out, err
}

func ffmpegPath() (string, error) {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	// GUI apps do not inherit the shell PATH.
	for _, p := range []string{"/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("需要 ffmpeg 才能播放过场（brew install ffmpeg）")
}

func (c *Cache) transcode(in, out string) error {
	ff, err := ffmpegPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	tmp := out + ".part.mp4"
	cmd := exec.Command(ff, "-y", "-loglevel", "error", "-i", in,
		"-c:v", "libx264", "-preset", "fast", "-crf", "18", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", tmp)
	if b, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("ffmpeg: %v: %s", err, b)
	}
	return os.Rename(tmp, out)
}
