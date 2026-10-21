package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"kikoku/internal/assets"
	"kikoku/internal/audio"
	"kikoku/internal/movie"
	"kikoku/internal/script"
)

const firstScene = "0101h.nss"

type Settings struct {
	GameDir    string  `json:"gameDir"`
	FontFamily string  `json:"fontFamily"`
	FontFile   string  `json:"fontFile"`
	FontScale  float64 `json:"fontScale"`
	LineHeight float64 `json:"lineHeight"`
	ParaGap    float64 `json:"paraGap"` // em between paragraphs
	BoxOpacity float64 `json:"boxOpacity"`
	Music      float64 `json:"music"`
	Voice      float64 `json:"voice"`
	Effect     float64 `json:"effect"`
	Last       int     `json:"last"`
}

func defaultSettings() Settings {
	home, _ := os.UserHomeDir()
	return Settings{
		GameDir:    filepath.Join(home, "Games", "kikoku-remake"),
		FontFamily: "Songti SC",
		FontScale:  1,
		LineHeight: 1.8,
		ParaGap:    0.6,
		BoxOpacity: 0.72,
		Music:      0.8,
		Voice:      1,
		Effect:     0.8,
	}
}

type App struct {
	ctx context.Context

	mu       sync.Mutex
	settings Settings
	store    *assets.Store
	book     *script.Book
	audio    *audio.Engine
	movies   *movie.Cache
	loadErr  error
	saveTick *time.Timer
}

func NewApp() *App { return &App{settings: defaultSettings()} }

func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "kikoku-reader")
}

func cacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "kikoku-reader")
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if b, err := os.ReadFile(filepath.Join(configDir(), "settings.json")); err == nil {
		s := defaultSettings()
		if json.Unmarshal(b, &s) == nil {
			a.settings = s
		}
	}
	a.loadErr = a.load(a.settings.GameDir)
}

// load opens the game folder and compiles the story.
func (a *App) load(dir string) error {
	store, err := assets.Open(dir)
	if err != nil {
		return err
	}
	files, err := script.Chain(store, firstScene)
	if err != nil {
		return err
	}
	book, err := script.Compile(store, files)
	if err != nil {
		return err
	}
	for _, w := range book.Warnings {
		log.Println("script:", w)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.store, a.book = store, book
	a.movies = movie.NewCache(filepath.Join(cacheDir(), "movies"), store.MoviePath)
	if a.audio == nil {
		if a.audio, err = audio.New(a.readAsset); err != nil {
			log.Println("audio:", err)
			a.audio = nil
		}
	}
	if a.audio != nil {
		a.audio.SetMaster(a.settings.Music, a.settings.Voice, a.settings.Effect)
	}
	return nil
}

func (a *App) readAsset(p string) ([]byte, error) {
	a.mu.Lock()
	s := a.store
	a.mu.Unlock()
	if s == nil {
		return nil, errors.New("no game data")
	}
	return s.ReadFile(p)
}

// domReady sizes the window to about 1440 pixels tall, within the screen.
func (a *App) domReady(ctx context.Context) {
	screens, err := runtime.ScreenGetAll(ctx)
	if err != nil || len(screens) == 0 {
		return
	}
	sc := screens[0]
	for _, s := range screens {
		if s.IsCurrent {
			sc = s
		}
	}
	h := sc.Size.Height * 88 / 100
	scale := 1
	if sc.PhysicalSize.Height > sc.Size.Height && sc.Size.Height > 0 {
		scale = sc.PhysicalSize.Height / sc.Size.Height
	}
	if h*scale > 1440 {
		h = 1440 / scale
	}
	w := h * 4 / 3
	if w > sc.Size.Width*95/100 {
		w = sc.Size.Width * 95 / 100
		h = w * 3 / 4
	}
	runtime.WindowSetSize(ctx, w, h)
	runtime.WindowCenter(ctx)
}

func (a *App) shutdown(ctx context.Context) { a.saveNow() }

type InitResult struct {
	Error    string         `json:"error,omitempty"`
	Scenes   []script.Scene `json:"scenes"`
	Total    int            `json:"total"`
	Settings Settings       `json:"settings"`
	Fonts    []FontChoice   `json:"fonts"`
}

type FontChoice struct {
	Label  string `json:"label"`
	Family string `json:"family"`
}

func (a *App) Init() InitResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := InitResult{Settings: a.settings, Fonts: presetFonts}
	if a.loadErr != nil {
		r.Error = a.loadErr.Error()
		return r
	}
	r.Scenes = a.book.Scenes
	r.Total = len(a.book.Steps)
	return r
}

var presetFonts = []FontChoice{
	{"宋体（Songti SC）", "Songti SC"},
	{"苹方（PingFang SC）", "PingFang SC"},
	{"楷体（Kaiti SC）", "Kaiti SC"},
	{"仿宋（STFangsong）", "STFangsong"},
	{"冬青黑体（Hiragino Sans GB）", "Hiragino Sans GB"},
	{"华文黑体（STHeiti）", "STHeiti"},
	{"兰亭黑（Lantinghei SC）", "Lantinghei SC"},
}

// ChooseGameDir lets the player point at the extracted remake folder.
func (a *App) ChooseGameDir() InitResult {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择《鬼哭街》重制版目录（含 nss.npa、cg.npa 的文件夹）",
	})
	if err != nil || dir == "" {
		return a.Init()
	}
	err = a.load(dir)
	a.mu.Lock()
	a.loadErr = err
	if err == nil {
		a.settings.GameDir = dir
		a.settings.Last = 0
	}
	a.mu.Unlock()
	a.saveSoon()
	return a.Init()
}

type View struct {
	Index int         `json:"index"`
	Step  script.Step `json:"step"`
	// Full text of the box: previous segments of the block plus this one.
	Before string   `json:"before"`
	Scene  int      `json:"scene"`
	Next   []string `json:"next"` // images of the next steps, for preloading
}

// Show returns step i for display and starts its sound.
func (a *App) Show(i int, jumped bool) (*View, error) {
	a.mu.Lock()
	book := a.book
	eng := a.audio
	a.mu.Unlock()
	if book == nil {
		return nil, errors.New("game not loaded")
	}
	if i < 0 || i >= len(book.Steps) {
		return nil, fmt.Errorf("step %d out of range", i)
	}
	st := book.Steps[i]
	v := &View{Index: i, Step: st}
	var before strings.Builder
	j := i
	for j > 0 && book.Steps[j].Append {
		j--
	}
	for k := j; k < i; k++ {
		before.WriteString(book.Steps[k].HTML)
	}
	v.Before = before.String()
	for n, sc := range book.Scenes {
		if sc.First <= i {
			v.Scene = n
		}
	}
	for k := i + 1; k < len(book.Steps) && k <= i+3; k++ {
		collectImages(book.Steps[k].Layers, &v.Next)
	}

	if eng != nil {
		want := make([]audio.Want, 0, len(st.Loops))
		for _, l := range st.Loops {
			want = append(want, audio.Want{Name: l.Name, Src: l.Src, Vol: l.Vol, Loop: true})
		}
		if jumped {
			eng.StopEffects()
		}
		eng.SetLoops(want)
		eng.PlayVoice(st.Voice)
		if !jumped {
			var se []audio.Want
			for _, s := range st.SE {
				se = append(se, audio.Want{Src: s.Src, Vol: s.Vol})
			}
			eng.PlayEffects(se)
		}
	}
	a.mu.Lock()
	a.settings.Last = i
	a.mu.Unlock()
	a.saveSoon()
	return v, nil
}

func collectImages(ls []script.Layer, out *[]string) {
	for _, l := range ls {
		if l.Kind == "image" {
			*out = append(*out, l.Src)
		}
		collectImages(l.Children, out)
	}
}

// StopVoice is called when the player hides the text or opens the menu.
func (a *App) StopVoice() {
	if a.audio != nil {
		a.audio.PlayVoice("")
	}
}

func (a *App) SaveSettings(s Settings) Settings {
	a.mu.Lock()
	s.GameDir = a.settings.GameDir
	s.Last = a.settings.Last
	a.settings = s
	eng := a.audio
	a.mu.Unlock()
	if eng != nil {
		eng.SetMaster(s.Music, s.Voice, s.Effect)
	}
	a.saveSoon()
	return s
}

// ChooseFontFile picks a .ttf/.otf/.ttc the page loads as "UserFont".
func (a *App) ChooseFontFile() string {
	p, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "选择字体文件",
		Filters: []runtime.FileFilter{{DisplayName: "字体 (*.ttf;*.otf;*.ttc)", Pattern: "*.ttf;*.otf;*.ttc;*.TTF;*.OTF;*.TTC"}},
	})
	if err != nil || p == "" {
		return ""
	}
	a.mu.Lock()
	a.settings.FontFile = p
	a.mu.Unlock()
	a.saveSoon()
	return p
}

func (a *App) ToggleFullscreen() {
	if runtime.WindowIsFullscreen(a.ctx) {
		runtime.WindowUnfullscreen(a.ctx)
	} else {
		runtime.WindowFullscreen(a.ctx)
	}
}

func (a *App) Quit() { runtime.Quit(a.ctx) }

func (a *App) saveSoon() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.saveTick != nil {
		a.saveTick.Stop()
	}
	a.saveTick = time.AfterFunc(800*time.Millisecond, a.saveNow)
}

func (a *App) saveNow() {
	a.mu.Lock()
	b, _ := json.MarshalIndent(a.settings, "", "  ")
	a.mu.Unlock()
	if err := os.MkdirAll(configDir(), 0o755); err == nil {
		_ = os.WriteFile(filepath.Join(configDir(), "settings.json"), b, 0o644)
	}
}

// ServeHTTP serves game resources to the page:
//
//	/res/<script path>     images and sounds from the NPA archives
//	/font/user             the font file chosen in settings
//	/movie/<dx path>.mp4   cutscenes, transcoded once with ffmpeg
type assetHandler struct{ a *App }

func (h assetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a := h.a
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/res/"):
		data, err := a.readAsset(strings.TrimPrefix(p, "/res/"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType(p))
		w.Header().Set("Cache-Control", "max-age=86400")
		_, _ = w.Write(data)
	case p == "/font/user":
		a.mu.Lock()
		f := a.settings.FontFile
		a.mu.Unlock()
		if f == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType(f))
		http.ServeFile(w, r, f)
	case strings.HasPrefix(p, "/movie/"):
		a.mu.Lock()
		mc := a.movies
		a.mu.Unlock()
		if mc == nil {
			http.NotFound(w, r)
			return
		}
		src := strings.TrimSuffix(strings.TrimPrefix(p, "/movie/"), ".mp4")
		out, err := mc.Get(src)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeFile(w, r, out)
	default:
		http.NotFound(w, r)
	}
}

func contentType(p string) string {
	ext := strings.ToLower(path.Ext(p))
	switch ext {
	case ".ogg":
		return "audio/ogg"
	case ".ttc":
		return "font/collection"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}
