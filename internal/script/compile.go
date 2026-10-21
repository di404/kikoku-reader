// Package script turns the remake's .nss scene files into a flat list of
// reading steps: one piece of text with the picture, voice and music that
// are current when it is shown. It tracks only the final state of each
// drawing call (no tweening, filters or camera work).
package script

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"kikoku/internal/assets"
)

const (
	ScreenW = 1024
	ScreenH = 768
)

// Layer is one drawn object in the 1024x768 logical screen.
type Layer struct {
	Kind     string  `json:"kind"` // image, color, window, movie
	Src      string  `json:"src,omitempty"`
	Color    string  `json:"color,omitempty"`
	Z        int     `json:"z"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
	Scale    float64 `json:"scale,omitempty"`
	ScaleY   float64 `json:"scaleY,omitempty"`
	Opacity  float64 `json:"opacity"`
	Blend    string  `json:"blend,omitempty"`
	Filter   string  `json:"filter,omitempty"`
	Loop     bool    `json:"loop,omitempty"`
	Children []Layer `json:"children,omitempty"`
}

type Sound struct {
	Name string  `json:"name"`
	Src  string  `json:"src"`
	Vol  float64 `json:"vol"` // 0..1
	Loop bool    `json:"loop"`
}

type Step struct {
	Scene   string  `json:"scene"`
	Label   string  `json:"label,omitempty"`
	HTML    string  `json:"html"`             // text of this segment, escaped
	Append  bool    `json:"append,omitempty"` // continues the previous step's text
	Speaker string  `json:"speaker,omitempty"`
	Side    string  `json:"side,omitempty"` // left/right half in the original layout
	Voice   string  `json:"voice,omitempty"`
	Layers  []Layer `json:"layers"`
	Loops   []Sound `json:"loops"`            // music and ambient loops playing
	SE      []Sound `json:"se,omitempty"`     // one-shots started since the previous step
	Movies  []Layer `json:"movies,omitempty"` // non-looping movies started here
}

type Scene struct {
	File  string `json:"file"`
	First int    `json:"first"` // index of its first step
	Title string `json:"title"`
}

type Book struct {
	Steps    []Step   `json:"-"`
	Scenes   []Scene  `json:"scenes"`
	Warnings []string `json:"-"`
}

// Chain follows $GameName from the first story scene, as 0_boot does.
func Chain(store *assets.Store, first string) ([]string, error) {
	re := regexp.MustCompile(`\$GameName\s*=\s*"([^"]+)"`)
	var out []string
	seen := map[string]bool{}
	for name := first; name != "" && !seen[name]; {
		seen[name] = true
		src, err := store.Script(name)
		if err != nil {
			return out, err
		}
		out = append(out, name)
		next := ""
		for _, m := range re.FindAllStringSubmatch(decodeSJIS(src), -1) {
			next = m[1]
		}
		name = next
	}
	return out, nil
}

func Compile(store *assets.Store, files []string) (*Book, error) {
	b := &Book{}
	c := &compiler{store: store, book: b}
	c.reset()
	for _, f := range files {
		src, err := store.Script(f)
		if err != nil {
			return nil, err
		}
		b.Scenes = append(b.Scenes, Scene{File: f, First: len(b.Steps), Title: sceneTitle(f)})
		c.scene = f
		for _, p := range split(src) {
			if p.pre != nil {
				c.pre(p.pre)
			} else {
				c.run(p.code)
			}
		}
	}
	return b, nil
}

func sceneTitle(f string) string {
	n := strings.TrimSuffix(f, ".nss")
	if len(n) >= 4 {
		return fmt.Sprintf("第%s章 · %s", strings.TrimLeft(n[:2], "0"), n[2:])
	}
	return n
}

type obj struct {
	name    string
	kind    string // image color window snap movie
	src     string
	color   string
	z       int
	x, y    float64
	w, h    float64
	sx, sy  float64
	opacity float64 // 0..1000
	blend   string
	filter  string
	loop    bool
	snap    []Layer
	seq     int
}

type sound struct {
	src  string
	vol  float64
	loop bool
}

type compiler struct {
	store *assets.Store
	book  *Book
	scene string

	objs    map[string]*obj
	seq     int
	stands  map[string]*obj // pending St() per id
	bgQueue []*obj          // OnBG waiting for FadeBG

	defs    map[string]string // sound name -> path
	vols    map[string]float64
	playing map[string]*sound // loops
	oneshot []Sound
	movies  []Layer

	// text state
	preStart       int // first step of the last <PRE> block
	speaker, voice string
	dirty          bool // visuals changed since last emitted step
}

func (c *compiler) reset() {
	c.objs = map[string]*obj{}
	c.stands = map[string]*obj{}
	c.defs = map[string]string{}
	c.vols = map[string]float64{}
	c.playing = map[string]*sound{}
}

func (c *compiler) warn(format string, a ...any) {
	if len(c.book.Warnings) < 500 {
		c.book.Warnings = append(c.book.Warnings, c.scene+": "+fmt.Sprintf(format, a...))
	}
}

func normName(s string) string { return strings.TrimPrefix(s, "@") }

func globRe(pat string) *regexp.Regexp {
	q := regexp.QuoteMeta(normName(pat))
	q = strings.ReplaceAll(q, `\*`, ".*")
	return regexp.MustCompile("^" + q + "$")
}

func (c *compiler) match(pat string) []*obj {
	pat = normName(pat)
	if !strings.Contains(pat, "*") {
		if o := c.objs[pat]; o != nil {
			return []*obj{o}
		}
		return nil
	}
	re := globRe(pat)
	var out []*obj
	for n, o := range c.objs {
		if re.MatchString(n) {
			out = append(out, o)
		}
	}
	return out
}

func (c *compiler) del(pat string) {
	for _, o := range c.match(pat) {
		delete(c.objs, o.name)
		// children go with their parent
		for n := range c.objs {
			if strings.HasPrefix(n, o.name+"/") {
				delete(c.objs, n)
			}
		}
	}
	c.dirty = true
}

// parentSize returns the box an object is positioned in.
func (c *compiler) parent(name string) *obj {
	if i := strings.LastIndexByte(name, '/'); i > 0 {
		if p := c.objs[name[:i]]; p != nil && p.kind == "window" {
			return p
		}
	}
	return nil
}

func (c *compiler) place(o *obj, ax, ay arg) {
	pw, ph := float64(ScreenW), float64(ScreenH)
	if p := c.parent(o.name); p != nil {
		pw, ph = p.w, p.h
	}
	o.x = position(ax, o.x, o.w, pw)
	o.y = position(ay, o.y, o.h, ph)
}

func position(a arg, cur, size, box float64) float64 {
	if v, ok := a.num(); ok {
		if a.rel() {
			return cur + v
		}
		return v
	}
	switch strings.ToLower(a.s) {
	case "center", "middle":
		return (box - size) / 2
	case "inleft", "intop":
		return 0
	case "inright", "inbottom":
		return box - size
	case "outleft", "outtop":
		return -size
	case "outright", "outbottom":
		return box
	case "onleft", "ontop":
		return -size / 2
	case "onright", "onbottom":
		return box - size/2
	}
	return cur
}

func (c *compiler) newObj(name, kind string, z int) *obj {
	name = normName(name)
	c.seq++
	o := &obj{name: name, kind: kind, z: z, sx: 1, sy: 1, seq: c.seq}
	c.objs[name] = o
	c.dirty = true
	return o
}

func (c *compiler) image(name string, z int, ax, ay arg, src string, visible bool, blend string) {
	if !c.store.Exists(src) {
		c.warn("找不到图片 %s", src)
		return
	}
	o := c.newObj(name, "image", z)
	o.src = src
	o.blend = blend
	w, h, err := c.store.ImageSize(src)
	if err != nil {
		c.warn("读不出尺寸 %s: %v", src, err)
	}
	o.w, o.h = float64(w), float64(h)
	c.place(o, ax, ay)
	if visible {
		o.opacity = 1000
	}
}

func colorValue(s string) string {
	switch strings.ToLower(s) {
	case "black", "white", "red", "blue", "green", "yellow", "gray", "grey":
		return strings.ToLower(s)
	}
	if strings.HasPrefix(s, "#") {
		return s
	}
	if len(s) == 6 {
		return "#" + s
	}
	return "black"
}

func blendOf(fn string) string {
	switch {
	case strings.HasSuffix(fn, "add"):
		return "add"
	case strings.HasSuffix(fn, "sub"):
		return "sub"
	case strings.HasSuffix(fn, "over"):
		return "overlay"
	case strings.HasSuffix(fn, "mul"):
		return "multiply"
	}
	return ""
}

func hidden(fn string) bool {
	base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(fn, "add"), "sub"), "over"), "mul")
	return strings.HasSuffix(base, "EX")
}

func zOf(a arg) int {
	v, _ := a.num()
	return int(v)
}

func (c *compiler) run(code string) {
	for _, k := range calls(code) {
		c.exec(k)
	}
}

func (c *compiler) exec(k call) {
	fn := k.name
	switch {
	case strings.HasPrefix(fn, "CreateTexture") && k.arg(4).s == "SCREEN":
		o := c.newObj(k.arg(0).s, "snap", zOf(k.arg(1)))
		o.snap = c.layers()
		o.w, o.h = ScreenW, ScreenH
		if !hidden(fn) {
			o.opacity = 1000
		}

	case strings.HasPrefix(fn, "CreateTexture"):
		if k.arg(4).isVar {
			return
		}
		c.image(k.arg(0).s, zOf(k.arg(1)), k.arg(2), k.arg(3), k.arg(4).s, !hidden(fn), blendOf(fn))

	case strings.HasPrefix(fn, "CreateColor"):
		o := c.newObj(k.arg(0).s, "color", zOf(k.arg(1)))
		o.blend = blendOf(fn)
		if len(k.args) >= 7 {
			x, _ := k.arg(2).num()
			y, _ := k.arg(3).num()
			w, _ := k.arg(4).num()
			h, _ := k.arg(5).num()
			o.x, o.y, o.w, o.h = x, y, w, h
			o.color = colorValue(k.arg(6).s)
		} else {
			o.w, o.h = ScreenW, ScreenH
			o.color = colorValue(k.args[len(k.args)-1].s)
		}
		if !hidden(fn) {
			o.opacity = 1000
		}

	case strings.HasPrefix(fn, "CreateWindow"):
		o := c.newObj(k.arg(0).s, "window", 0)
		x, _ := k.arg(1).num()
		y, _ := k.arg(2).num()
		w, _ := k.arg(3).num()
		h, _ := k.arg(4).num()
		o.x, o.y, o.w, o.h = x, y, w, h
		o.opacity = 1000

	case strings.HasPrefix(fn, "CreatePlain"), fn == "PrintGO", fn == "PrintBG":
		snap := c.layers()
		if fn == "PrintBG" || fn == "PrintGO" {
			c.objs = map[string]*obj{}
		}
		o := c.newObj(k.arg(0).s, "snap", zOf(k.arg(1)))
		o.snap = snap
		o.w, o.h = ScreenW, ScreenH
		o.opacity = 1000

	case strings.HasPrefix(fn, "CreateMovie"):
		// name, z, x, y, loop, alpha, path
		src := k.arg(6).s
		o := c.newObj(k.arg(0).s, "movie", zOf(k.arg(1)))
		o.src = src
		o.w, o.h = ScreenW, ScreenH
		o.loop = k.arg(4).s == "true"
		c.place(o, k.arg(2), k.arg(3))
		if fn == "CreateMovie" {
			o.opacity = 1000
		}
		if !o.loop && storyMovie(src) {
			c.movies = append(c.movies, Layer{Kind: "movie", Src: src, Z: o.z})
		}

	case fn == "Fade" || fn == "DrawTransition" || strings.HasPrefix(fn, "FadeFR"):
		idx := 2
		if fn == "DrawTransition" {
			idx = 3
		}
		v, ok := k.arg(idx).num()
		if !ok {
			return
		}
		if fn == "DrawTransition" && v > 0 && k.arg(6).s != "" && k.arg(6).s != "null" {
			v = 1000 // partial rule wipes still show the object
		}
		for _, o := range c.match(k.arg(0).s) {
			o.opacity = v
		}
		c.dirty = true

	case fn == "FadeDelete" || fn == "Delete":
		c.del(k.arg(0).s)
		if p := k.arg(0).s; p != "*" && p != "@*" {
			c.stopSound(p)
		}

	case fn == "Zoom":
		sx, ok1 := k.arg(2).num()
		sy, ok2 := k.arg(3).num()
		if !ok1 || !ok2 {
			return
		}
		for _, o := range c.match(k.arg(0).s) {
			o.sx, o.sy = sx/1000, sy/1000
		}
		c.dirty = true

	case fn == "Move":
		for _, o := range c.match(k.arg(0).s) {
			if _, ok := k.arg(2).num(); ok {
				o.x = position(k.arg(2), o.x, o.w, ScreenW)
			}
			if _, ok := k.arg(3).num(); ok {
				o.y = position(k.arg(3), o.y, o.h, ScreenH)
			}
		}
		c.dirty = true

	case fn == "SetTone":
		f := ""
		switch strings.ToLower(k.arg(1).s) {
		case "monochrome":
			f = "grayscale(1)"
		case "sepia":
			f = "sepia(0.8)"
		case "negative", "negaposi":
			f = "invert(1)"
		}
		for _, o := range c.match(k.arg(0).s) {
			o.filter = f
		}

	case fn == "OnBG":
		name := k.arg(1).s
		src := "cg/bg/" + name + ".jpg"
		if !c.store.Exists(src) {
			c.warn("找不到背景 %s", src)
			return
		}
		o := &obj{kind: "image", src: src, z: zOf(k.arg(0)), sx: 1, sy: 1}
		w, h, _ := c.store.ImageSize(src)
		o.w, o.h = float64(w), float64(h)
		o.x, o.y = (ScreenW-o.w)/2, (ScreenH-o.h)/2
		c.bgQueue = append(c.bgQueue, o)

	case fn == "FadeBG":
		if len(c.bgQueue) == 0 {
			return
		}
		o := c.bgQueue[0]
		c.bgQueue = c.bgQueue[1:]
		c.del("OnBG*")
		c.seq++
		o.name, o.seq, o.opacity = fmt.Sprintf("OnBG%d", c.seq), c.seq, 1000
		c.objs[o.name] = o
		c.dirty = true

	case fn == "St" || fn == "StF":
		id := k.arg(0).s
		img := k.arg(4).s
		if k.arg(4).isVar {
			return
		}
		folder := "st/"
		for _, p := range []string{"bu", "fu", "tv"} {
			if strings.HasPrefix(img, p) {
				folder = p + "/"
			}
		}
		src := "cg/" + folder + img + ".png"
		if !c.store.Exists(src) {
			c.warn("找不到立绘 %s", src)
			return
		}
		w, h, _ := c.store.ImageSize(src)
		o := &obj{kind: "image", src: src, z: zOf(k.arg(1)), sx: 1, sy: 1, w: float64(w), h: float64(h)}
		o.x = (ScreenW-o.w)/2 + standOffset[id]
		o.y = ScreenH - o.h
		c.stands[id] = o

	case fn == "FadeSt":
		id := k.arg(0).s
		if o := c.stands[id]; o != nil {
			delete(c.stands, id)
			c.seq++
			o.name, o.seq, o.opacity = "StName"+id, c.seq, 1000
			c.objs[o.name] = o
			c.dirty = true
		}

	case fn == "DeleteSt":
		c.del("StName" + k.arg(0).s)

	case fn == "DeleteAllSt":
		c.del("StName*")

	case fn == "CreateSE" && k.arg(1).isVar, fn == "OnSE" && k.arg(0).isVar:
		// sound chosen at run time from a script variable; nothing to play
	case fn == "CreateSE":
		c.defs[normName(k.arg(0).s)] = "sound/se/" + k.arg(1).s
	case fn == "CreateVOICE":
		c.defs[normName(k.arg(0).s)] = "voice/" + k.arg(1).s
	case fn == "CreateSound":
		c.defs[normName(k.arg(0).s)] = k.arg(2).s
	case fn == "OnSE":
		v, _ := k.arg(1).num()
		c.oneshot = append(c.oneshot, Sound{Src: c.soundSrc("sound/se/" + k.arg(0).s), Vol: v / 1000})

	case fn == "SoundPlay" || fn == "MusicStart":
		name := normName(k.arg(0).s)
		src := c.defs[name]
		if src == "" && bgmName.MatchString(name) {
			src = "sound/bgm/" + name
		}
		if src == "" {
			c.warn("未定义的声音 %s", name)
			return
		}
		src = c.soundSrc(src)
		v, ok := k.arg(2).num()
		if !ok {
			v = 1000
		}
		loopArg := k.arg(3)
		if fn == "MusicStart" {
			loopArg = k.arg(6)
		}
		if loopArg.s == "true" {
			c.playing[name] = &sound{src: src, vol: v / 1000, loop: true}
		} else {
			c.oneshot = append(c.oneshot, Sound{Name: name, Src: src, Vol: v / 1000})
		}

	case fn == "SetVolume" || fn == "SetVolumeEX":
		v, ok := k.arg(2).num()
		if !ok {
			return
		}
		re := globRe(k.arg(0).s)
		for n, s := range c.playing {
			if re.MatchString(n) {
				if v <= 0 {
					delete(c.playing, n)
				} else {
					s.vol = v / 1000
				}
			}
		}

	case fn == "Request":
		if strings.EqualFold(k.arg(1).s, "Stop") {
			c.stopSound(k.arg(0).s)
		}

	case fn == "SetTextL" || fn == "SetTextR" || fn == "SetText":
		// Comes after the <PRE> block it lays out.
		side := map[string]string{"SetTextL": "left", "SetTextR": "right", "SetText": ""}[fn]
		for i := c.preStart; i < len(c.book.Steps); i++ {
			c.book.Steps[i].Side = side
		}

	case fn == "WaitKey":
		if c.dirty {
			c.emit("", false)
		}
	}
}

// Only mv01xx/mv02xx are cutscenes; the rest are overlay effects (speed
// lines, sparks, smoke) that need the engine's compositing.
func storyMovie(src string) bool {
	return strings.HasPrefix(src, "dx/mv01") || strings.HasPrefix(src, "dx/mv02")
}

var bgmName = regexp.MustCompile(`^(m\d|uta)`)

var standOffset = map[string]float64{"L": -300, "R": 300, "ML": -150, "MR": 150, "L2": -300, "R2": 300}

func (c *compiler) soundSrc(p string) string { return c.store.Canonical(p) }

func (c *compiler) stopSound(pat string) {
	re := globRe(pat)
	for n := range c.playing {
		if re.MatchString(n) {
			delete(c.playing, n)
		}
	}
}

// layers renders the current object set, sorted by z then creation order.
func (c *compiler) layers() []Layer {
	var top []*obj
	kids := map[string][]*obj{}
	for _, o := range c.objs {
		if p := c.parent(o.name); p != nil {
			kids[p.name] = append(kids[p.name], o)
		} else {
			top = append(top, o)
		}
	}
	// windows have no z of their own; they sit at their lowest child's
	for _, o := range top {
		if o.kind != "window" {
			continue
		}
		for i, kid := range kids[o.name] {
			if i == 0 || kid.z < o.z {
				o.z = kid.z
			}
		}
	}
	var conv func(list []*obj) []Layer
	conv = func(list []*obj) []Layer {
		sort.Slice(list, func(i, j int) bool {
			if list[i].z != list[j].z {
				return list[i].z < list[j].z
			}
			return list[i].seq < list[j].seq
		})
		var out []Layer
		for _, o := range list {
			if o.opacity <= 0 || o.blend == "sub" {
				continue
			}
			l := Layer{Kind: o.kind, Z: o.z, X: o.x, Y: o.y, W: o.w, H: o.h,
				Opacity: o.opacity / 1000, Blend: o.blend, Filter: o.filter, Color: o.color, Loop: o.loop}
			if o.sx != 1 || o.sy != 1 {
				l.Scale, l.ScaleY = o.sx, o.sy
			}
			switch o.kind {
			case "image":
				l.Src = c.store.HiRes(o.src)
			case "movie":
				l.Src = o.src
				if !o.loop {
					continue // one-shot movies are played separately
				}
			case "snap":
				l.Kind = "group"
				l.Children = o.snap
			case "window":
				l.Children = conv(kids[o.name])
				if len(l.Children) == 0 {
					continue
				}
			}
			out = append(out, l)
		}
		return out
	}
	return conv(top)
}

func (c *compiler) emit(text string, appendText bool) {
	st := Step{
		Scene:   c.scene,
		HTML:    text,
		Append:  appendText,
		Speaker: speakerName(c.speaker),
		Voice:   c.voice,
		Layers:  c.layers(),
		SE:      c.oneshot,
		Movies:  c.movies,
	}
	names := make([]string, 0, len(c.playing))
	for n := range c.playing {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := c.playing[n]
		st.Loops = append(st.Loops, Sound{Name: n, Src: s.src, Vol: s.vol, Loop: true})
	}
	c.oneshot, c.movies = nil, nil
	c.voice, c.speaker = "", ""
	c.dirty = false
	c.book.Steps = append(c.book.Steps, st)
}

var blankLines = regexp.MustCompile(`\n(\s*\n)+`)

var rubyRe = regexp.MustCompile(`<RUBY text="([^"]*)">(.*?)</RUBY>`)

// textHTML escapes a text segment, turns <RUBY text="reading">base</RUBY>
// into an HTML ruby annotation and {Wait(ms);} into a pause marker.
func textHTML(s string) string {
	var b strings.Builder
	last := 0
	for _, m := range rubyRe.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(html.EscapeString(s[last:m[0]]))
		fmt.Fprintf(&b, "<ruby>%s<rt>%s</rt></ruby>",
			html.EscapeString(s[m[4]:m[5]]), html.EscapeString(s[m[2]:m[3]]))
		last = m[1]
	}
	b.WriteString(html.EscapeString(s[last:]))
	return inlineCode.ReplaceAllStringFunc(b.String(), func(m string) string {
		if w := waitRe.FindStringSubmatch(m); w != nil {
			return `<i class="wait" data-ms="` + w[1] + `"></i>`
		}
		return ""
	})
}

// Code embedded mid-line; {Wait(ms);} pauses the typing for a beat.
var (
	inlineCode = regexp.MustCompile(`\{[^{}]*\}`)
	waitRe     = regexp.MustCompile(`^\{\s*Wait\((\d+)\);?\s*\}$`)
)

func (c *compiler) pre(pb *preBlock) {
	var buf strings.Builder
	first := true
	c.preStart = len(c.book.Steps)
	flush := func(final bool) {
		t := buf.String()
		buf.Reset()
		if strings.TrimSpace(strings.ReplaceAll(t, "　", "")) == "" {
			return
		}
		if first {
			t = strings.TrimLeft(t, "\n")
		}
		if final {
			t = strings.TrimRight(t, "\n")
		}
		t = blankLines.ReplaceAllString(t, "\n\n")
		c.emit(textHTML(t), !first)
		c.book.Steps[len(c.book.Steps)-1].Label = pb.label
		first = false
	}
	for _, it := range pb.items {
		switch it.kind {
		case "text":
			buf.WriteString(it.text)
		case "voice":
			// Each voiced line gets its own click, as in the original.
			if strings.TrimSpace(strings.ReplaceAll(buf.String(), "\u3000", "")) != "" {
				flush(false)
			}
			if it.voice != "" {
				c.voice = c.store.Canonical(it.voice)
				if !c.store.Exists(c.voice) {
					c.warn("找不到语音 %s", it.voice)
					c.voice = ""
				}
			}
			c.speaker = it.name
		case "wait":
			// text after <k> continues on the same line in the original
			flush(false)
		case "code":
			flush(false)
			c.run(it.text)
		}
	}
	flush(true)
}
