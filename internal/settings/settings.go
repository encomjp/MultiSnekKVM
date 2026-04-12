package settings

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Settings holds all user-configurable preferences that persist across restarts.
type Settings struct {
	EdgeSide       string  `json:"edgeSide"`
	Sensitivity    float64 `json:"sensitivity"`
	AudioMode      string  `json:"audioMode"`
	AudioTiming    string  `json:"audioTiming"`
	AudioTransport string  `json:"audioTransport"`
	AudioProfile   string  `json:"audioProfile"`
	Autostart      bool    `json:"autostart"`
	StartMinimized bool    `json:"startMinimized"`

	MuteSource bool   `json:"muteSource"`
	MicMode    string `json:"micMode"`

	CaptureDeviceID     string `json:"captureDeviceId,omitempty"`
	PlaybackDeviceID    string `json:"playbackDeviceId,omitempty"`
	MicDeviceID         string `json:"micDeviceId,omitempty"`
	MicPlaybackDeviceID string `json:"micPlaybackDeviceId,omitempty"`

	LastPeerID   string            `json:"lastPeerID,omitempty"`
	LastPeerName string            `json:"lastPeerName,omitempty"`
	LastPeerAddr map[string]string `json:"lastPeerAddr,omitempty"`

	AutoReconnect *bool `json:"autoReconnect,omitempty"`

	// Exit hotkey. ExitVKCode==0 means ESC (always active as fallback).
	// ExitModifiers bitmask: 1=Ctrl, 2=Alt, 4=Shift, 8=Win.
	ExitVKCode    uint16 `json:"exitVKCode,omitempty"`
	ExitModifiers uint8  `json:"exitModifiers,omitempty"`

	// Trigger zone — edge segment that activates remote mode.
	// If TriggerMonitorID=="", legacy full-edge detection is used.
	TriggerMonitorID string  `json:"triggerMonitorId,omitempty"`
	TriggerSide      string  `json:"triggerSide,omitempty"`
	TriggerStartPct  float32 `json:"triggerStartPct,omitempty"`
	TriggerEndPct    float32 `json:"triggerEndPct,omitempty"`

	// Return anchor — where the cursor lands after exiting remote mode.
	// If ReturnMonitorID=="", legacy center-of-primary logic is used.
	ReturnMonitorID string  `json:"returnMonitorId,omitempty"`
	ReturnXPct      float32 `json:"returnXPct,omitempty"`
	ReturnYPct      float32 `json:"returnYPct,omitempty"`
}

var defaultSettings = Settings{
	EdgeSide:       "right",
	Sensitivity:    1.0,
	AudioMode:      "off",
	AudioTiming:    "always",
	AudioTransport: "pcm",
	AudioProfile:   "balanced",
	MicMode:        "off",
	Autostart:      false,
	StartMinimized: false,
}

type Store struct {
	mu   sync.Mutex
	path string
	data Settings
}

func copyLastPeerAddr(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for route, addr := range src {
		dst[route] = addr
	}
	return dst
}

func NewStore() *Store {
	dir, err := appDataDir()
	if err != nil {
		log.Printf("settings: cannot determine app data dir: %v", err)
		return &Store{data: defaultSettings}
	}
	s := &Store{
		path: filepath.Join(dir, "settings.json"),
		data: defaultSettings,
	}
	s.load()
	return s
}

func (s *Store) load() {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var loaded Settings
	if err := json.Unmarshal(raw, &loaded); err != nil {
		log.Printf("settings: parse error: %v", err)
		return
	}
	if loaded.EdgeSide != "" {
		s.data.EdgeSide = loaded.EdgeSide
	}
	if loaded.Sensitivity >= 0.1 && loaded.Sensitivity <= 5.0 {
		s.data.Sensitivity = loaded.Sensitivity
	}
	if loaded.AudioMode != "" {
		s.data.AudioMode = loaded.AudioMode
	}
	if loaded.AudioTiming != "" {
		s.data.AudioTiming = loaded.AudioTiming
	}
	if loaded.AudioTransport != "" {
		s.data.AudioTransport = loaded.AudioTransport
	}
	if loaded.AudioProfile != "" {
		s.data.AudioProfile = loaded.AudioProfile
	}
	if loaded.MicMode != "" {
		s.data.MicMode = loaded.MicMode
	}
	s.data.CaptureDeviceID = loaded.CaptureDeviceID
	s.data.PlaybackDeviceID = loaded.PlaybackDeviceID
	s.data.MicDeviceID = loaded.MicDeviceID
	s.data.MicPlaybackDeviceID = loaded.MicPlaybackDeviceID
	s.data.MuteSource = loaded.MuteSource
	s.data.Autostart = loaded.Autostart
	s.data.StartMinimized = loaded.StartMinimized
	s.data.LastPeerID = loaded.LastPeerID
	s.data.LastPeerName = loaded.LastPeerName
	s.data.AutoReconnect = loaded.AutoReconnect
	s.data.LastPeerAddr = copyLastPeerAddr(loaded.LastPeerAddr)

	// Exit hotkey
	s.data.ExitVKCode = loaded.ExitVKCode
	s.data.ExitModifiers = loaded.ExitModifiers

	// Trigger zone
	s.data.TriggerMonitorID = loaded.TriggerMonitorID
	s.data.TriggerSide = loaded.TriggerSide
	if loaded.TriggerEndPct > 0 {
		s.data.TriggerStartPct = loaded.TriggerStartPct
		s.data.TriggerEndPct = loaded.TriggerEndPct
	}

	// Return anchor
	s.data.ReturnMonitorID = loaded.ReturnMonitorID
	if loaded.ReturnMonitorID != "" {
		s.data.ReturnXPct = loaded.ReturnXPct
		s.data.ReturnYPct = loaded.ReturnYPct
	}
}

func (s *Store) save() {
	if s.path == "" {
		return
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		log.Printf("settings: marshal error: %v", err)
		return
	}
	if err := os.WriteFile(s.path, raw, 0o600); err != nil {
		log.Printf("settings: write error: %v", err)
	}
}

func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	copySettings := s.data
	copySettings.LastPeerAddr = copyLastPeerAddr(s.data.LastPeerAddr)
	return copySettings
}

func (s *Store) Update(fn func(*Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.data)
	s.save()
}

func appDataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "MultiSnekKVM")
	return path, os.MkdirAll(path, 0o700)
}
