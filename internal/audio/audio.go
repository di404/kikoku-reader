// Package audio plays the game's Ogg Vorbis music, voices and effects.
// All game audio is 44.1 kHz; mono is duplicated to both channels.
package audio

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"sync"

	"github.com/ebitengine/oto/v3"
	"github.com/jfreymuth/oggvorbis"
)

const rate = 44100

type Loader func(path string) ([]byte, error)

type Category int

const (
	Music Category = iota
	Voice
	Effect
)

// Want describes a sound that should be playing.
type Want struct {
	Name string
	Src  string
	Vol  float64
	Loop bool
}

type Engine struct {
	ctx  *oto.Context
	load Loader

	mu     sync.Mutex
	master [3]float64
	loops  map[string]*track // key: name
	voice  *track
	shots  []*track
}

type track struct {
	src    string
	cat    Category
	vol    float64
	player *oto.Player
	s      *stream
}

func New(load Loader) (*Engine, error) {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   rate,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
	})
	if err != nil {
		return nil, err
	}
	<-ready
	return &Engine{ctx: ctx, load: load, master: [3]float64{1, 1, 1}, loops: map[string]*track{}}, nil
}

func (e *Engine) SetMaster(music, voice, effect float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.master = [3]float64{music, voice, effect}
	for _, t := range e.loops {
		e.applyVol(t)
	}
	if e.voice != nil {
		e.applyVol(e.voice)
	}
}

func (e *Engine) applyVol(t *track) {
	// Script volumes are linear 0..1; perceived loudness wants a curve.
	v := t.vol * e.master[t.cat]
	t.s.setGain(v*v, 0.25)
}

func (e *Engine) start(src string, cat Category, vol float64, loop bool) *track {
	data, err := e.load(src)
	if err != nil {
		return nil
	}
	s, err := newStream(data, loop)
	if err != nil {
		return nil
	}
	t := &track{src: src, cat: cat, vol: vol, s: s}
	g := vol * e.master[cat]
	s.gain, s.target = g*g, g*g
	if loop {
		// fade music in instead of starting at full level
		s.gain = 0
		s.setGain(g*g, 1.0)
	}
	t.player = e.ctx.NewPlayer(s)
	t.player.SetBufferSize(rate * 2 * 4 / 10)
	t.player.Play()
	return t
}

func (e *Engine) stop(t *track, fade float64) {
	if t == nil {
		return
	}
	t.s.fadeOut(fade)
}

// SetLoops makes exactly the given loops play, keeping ones already running.
func (e *Engine) SetLoops(want []Want) {
	e.mu.Lock()
	defer e.mu.Unlock()
	keep := map[string]bool{}
	for _, w := range want {
		keep[w.Name] = true
		if t := e.loops[w.Name]; t != nil && t.src == w.Src && !t.s.stopping() {
			if t.vol != w.Vol {
				t.vol = w.Vol
				e.applyVol(t)
			}
			continue
		} else if t != nil {
			e.stop(t, 0.8)
		}
		cat := Effect
		if isMusic(w.Src) {
			cat = Music
		}
		if t := e.start(w.Src, cat, w.Vol, true); t != nil {
			e.loops[w.Name] = t
		} else {
			delete(e.loops, w.Name)
		}
	}
	for n, t := range e.loops {
		if !keep[n] {
			e.stop(t, 1.2)
			delete(e.loops, n)
		}
	}
}

func isMusic(src string) bool { return bytes.Contains([]byte(src), []byte("/bgm/")) }

// PlayVoice stops the previous voice and starts a new one ("" just stops).
func (e *Engine) PlayVoice(src string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stop(e.voice, 0.08)
	e.voice = nil
	if src != "" {
		e.voice = e.start(src, Voice, 1, false)
	}
}

func (e *Engine) PlayEffects(list []Want) {
	e.mu.Lock()
	defer e.mu.Unlock()
	live := e.shots[:0]
	for _, t := range e.shots {
		if t.player.IsPlaying() {
			live = append(live, t)
		}
	}
	e.shots = live
	for _, w := range list {
		if t := e.start(w.Src, Effect, w.Vol, false); t != nil {
			e.shots = append(e.shots, t)
		}
	}
}

// StopEffects cuts one-shot effects, used when jumping around.
func (e *Engine) StopEffects() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range e.shots {
		e.stop(t, 0.1)
	}
	e.shots = nil
}

// stream decodes Ogg Vorbis on demand into interleaved stereo float32.
type stream struct {
	mu     sync.Mutex
	dec    *oggvorbis.Reader
	chans  int
	loop   bool
	buf    []float32
	gain   float64
	target float64
	step   float64 // gain change per frame
	stop   bool
	done   bool
}

func newStream(data []byte, loop bool) (*stream, error) {
	d, err := oggvorbis.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &stream{dec: d, chans: d.Channels(), loop: loop}, nil
}

func (s *stream) setGain(g, seconds float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.target = g
	if seconds <= 0 {
		s.gain = g
		s.step = 0
		return
	}
	s.step = math.Abs(g-s.gain) / (seconds * rate)
}

func (s *stream) fadeOut(seconds float64) {
	s.setGain(0, seconds)
	s.mu.Lock()
	s.stop = true
	s.mu.Unlock()
}

func (s *stream) stopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stop
}

func (s *stream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return 0, io.EOF
	}
	frames := len(p) / 8
	if frames == 0 {
		return 0, nil
	}
	need := frames * s.chans
	if cap(s.buf) < need {
		s.buf = make([]float32, need)
	}
	buf := s.buf[:need]
	got := 0
	for got < need {
		n, err := s.dec.Read(buf[got:])
		got += n
		if err == io.EOF {
			if s.loop && s.dec.SetPosition(0) == nil {
				continue
			}
			break
		}
		if err != nil {
			break
		}
	}
	gotFrames := got / s.chans
	for f := 0; f < gotFrames; f++ {
		if s.gain != s.target {
			if s.gain < s.target {
				s.gain = math.Min(s.target, s.gain+s.step)
			} else {
				s.gain = math.Max(s.target, s.gain-s.step)
			}
			if s.step == 0 {
				s.gain = s.target
			}
		}
		l := buf[f*s.chans]
		r := l
		if s.chans > 1 {
			r = buf[f*s.chans+1]
		}
		g := float32(s.gain)
		binary.LittleEndian.PutUint32(p[f*8:], math.Float32bits(l*g))
		binary.LittleEndian.PutUint32(p[f*8+4:], math.Float32bits(r*g))
	}
	if s.stop && s.gain <= 0 {
		s.done = true
	}
	if gotFrames == 0 {
		s.done = true
		return 0, io.EOF
	}
	return gotFrames * 8, nil
}
