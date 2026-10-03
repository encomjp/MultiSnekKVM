package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateWritesAtomicallyAndSkipsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := &Store{path: path, data: defaultSettings}

	s.Update(func(cfg *Settings) { cfg.EdgeSide = "left" })
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Settings
	if err := json.Unmarshal(raw, &got); err != nil || got.EdgeSide != "left" {
		t.Fatalf("saved settings = %+v, %v", got, err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file must not be left behind")
	}

	// An update that changes nothing must not rewrite the file.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	s.Update(func(cfg *Settings) { cfg.EdgeSide = "left" })
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Fatal("unchanged update rewrote the settings file")
	}

	s.Update(func(cfg *Settings) { cfg.Sensitivity = 2 })
	reloaded := &Store{path: path, data: defaultSettings}
	reloaded.load()
	if reloaded.Get().Sensitivity != 2 || reloaded.Get().EdgeSide != "left" {
		t.Fatalf("reloaded = %+v", reloaded.Get())
	}
}
