package trust

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type TrustedPeer struct {
	DeviceID     string    `json:"device_id"`
	Name         string    `json:"name"`
	Fingerprint  string    `json:"fingerprint"`
	PairedAt     time.Time `json:"paired_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	LastEndpoint string    `json:"last_endpoint"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	records map[string]TrustedPeer
}

func OpenStore() (*Store, error) {
	root, err := appDataDir()
	if err != nil {
		return nil, err
	}

	trustDir := filepath.Join(root, "trust")
	if err := os.MkdirAll(trustDir, 0o700); err != nil {
		return nil, fmt.Errorf("create trust dir: %w", err)
	}

	store := &Store{
		path:    filepath.Join(trustDir, "peers.json"),
		records: make(map[string]TrustedPeer),
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (store *Store) GetByDeviceID(deviceID string) (TrustedPeer, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	record, ok := store.records[deviceID]
	return record, ok
}

func (store *Store) IsTrusted(deviceID, fingerprint string) bool {
	record, ok := store.GetByDeviceID(deviceID)
	if !ok {
		return false
	}
	return strings.EqualFold(record.Fingerprint, fingerprint)
}

func (store *Store) Upsert(record TrustedPeer) error {
	if strings.TrimSpace(record.DeviceID) == "" || strings.TrimSpace(record.Fingerprint) == "" {
		return fmt.Errorf("device id and fingerprint are required")
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	existing, ok := store.records[record.DeviceID]
	if ok && !existing.PairedAt.IsZero() {
		record.PairedAt = existing.PairedAt
	} else if record.PairedAt.IsZero() {
		record.PairedAt = time.Now().UTC()
	}
	if record.LastSeenAt.IsZero() {
		record.LastSeenAt = time.Now().UTC()
	}

	store.records[record.DeviceID] = record
	return store.saveLocked()
}

func (store *Store) MarkSeen(deviceID, endpoint string, seenAt time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	record, ok := store.records[deviceID]
	if !ok {
		return nil
	}
	record.LastSeenAt = seenAt.UTC()
	if endpoint != "" {
		record.LastEndpoint = endpoint
	}
	store.records[deviceID] = record
	return store.saveLocked()
}

func (store *Store) Remove(deviceID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if _, ok := store.records[deviceID]; !ok {
		return nil
	}
	delete(store.records, deviceID)
	return store.saveLocked()
}

func (store *Store) load() error {
	data, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read trust store: %w", err)
	}

	var records []TrustedPeer
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("decode trust store: %w", err)
	}

	for _, record := range records {
		if strings.TrimSpace(record.DeviceID) == "" {
			continue
		}
		store.records[record.DeviceID] = record
	}

	return nil
}

func (store *Store) saveLocked() error {
	records := make([]TrustedPeer, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].DeviceID < records[j].DeviceID
	})

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trust store: %w", err)
	}

	tmpPath := store.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write trust store temp: %w", err)
	}
	if err := os.Rename(tmpPath, store.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace trust store: %w", err)
	}

	return nil
}

func appDataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "MultiSnekKVM")
	return path, os.MkdirAll(path, 0o700)
}
