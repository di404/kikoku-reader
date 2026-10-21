// playtest plays a few seconds of music and a voice through the engine.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"kikoku/internal/assets"
	"kikoku/internal/audio"
)

func main() {
	home, _ := os.UserHomeDir()
	store, err := assets.Open(filepath.Join(home, "Games", "kikoku-remake"))
	if err != nil {
		panic(err)
	}
	e, err := audio.New(store.ReadFile)
	if err != nil {
		panic(err)
	}
	e.SetLoops([]audio.Want{{Name: "m06", Src: "sound/bgm/m06.ogg", Vol: 0.45, Loop: true}})
	time.Sleep(1500 * time.Millisecond)
	e.PlayVoice("voice/01/0200010a01.ogg")
	e.PlayEffects([]audio.Want{{Src: "sound/se/cyborg_00_arm01.ogg", Vol: 0.7}})
	time.Sleep(2500 * time.Millisecond)
	e.SetLoops(nil)
	time.Sleep(1500 * time.Millisecond)
	fmt.Println("ok")
}
