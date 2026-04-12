package logutil

import (
	"strings"
	"testing"
)

func TestAnalyzeRecentLogs_empty(t *testing.T) {
	// Temporarily replace the ring with an empty snapshot.
	recentLogsMu.Lock()
	saved := recentLogsRing
	recentLogsRing = nil
	recentLogsMu.Unlock()
	defer func() {
		recentLogsMu.Lock()
		recentLogsRing = saved
		recentLogsMu.Unlock()
	}()

	a := AnalyzeRecentLogs()
	if len(a.Events) != 0 {
		t.Errorf("expected 0 events for empty log ring, got %d", len(a.Events))
	}
	if a.TotalErrors != 0 {
		t.Errorf("expected 0 total errors, got %d", a.TotalErrors)
	}
	if a.WindowLines != 0 {
		t.Errorf("expected 0 window lines, got %d", a.WindowLines)
	}
}

func TestAnalyzeRecentLogs_detects_patterns(t *testing.T) {
	recentLogsMu.Lock()
	saved := recentLogsRing
	recentLogsRing = []string{
		"2024/01/01 PANIC in goroutine send-mux: runtime error",
		"2024/01/01 send-mux STALL: no send for 600ms with frames queued H:5/M:0/F:0",
		"2024/01/01 send frame 0x01 failed: i/o timeout",
		"2024/01/01 auto-reconnect: attempt 2 to MyPC via 1 candidates",
		"2024/01/01 [log-watchdog] pattern=panic level=critical count=1", // must be ignored
	}
	recentLogsMu.Unlock()
	defer func() {
		recentLogsMu.Lock()
		recentLogsRing = saved
		recentLogsMu.Unlock()
	}()

	a := AnalyzeRecentLogs()

	byID := make(map[string]LogEvent)
	for _, ev := range a.Events {
		byID[ev.Pattern] = ev
	}

	if ev, ok := byID["panic"]; !ok {
		t.Error("expected panic pattern detected")
	} else if ev.Level != "critical" {
		t.Errorf("panic level: want critical, got %s", ev.Level)
	}

	if ev, ok := byID["send-stall"]; !ok {
		t.Error("expected send-stall pattern detected")
	} else if ev.Level != "error" {
		t.Errorf("send-stall level: want error, got %s", ev.Level)
	}

	if ev, ok := byID["io-timeout"]; !ok {
		t.Error("expected io-timeout pattern detected")
	} else if ev.Level != "error" {
		t.Errorf("io-timeout level: want error, got %s", ev.Level)
	}

	if ev, ok := byID["reconnect-attempt"]; !ok {
		t.Error("expected reconnect-attempt pattern detected")
	} else if ev.Level != "info" {
		t.Errorf("reconnect-attempt level: want info, got %s", ev.Level)
	}

	// Watchdog line must NOT be re-detected.
	for _, ev := range a.Events {
		if strings.Contains(ev.Sample, "[log-watchdog]") {
			t.Errorf("watchdog self-detection: found sample containing [log-watchdog]: %s", ev.Sample)
		}
	}
}

func TestAnalyzeRecentLogs_countAccumulation(t *testing.T) {
	recentLogsMu.Lock()
	saved := recentLogsRing
	recentLogsRing = []string{
		"2024/01/01 send frame 0x01 failed: connection refused",
		"2024/01/01 send frame 0x02 failed: connection refused",
		"2024/01/01 send frame 0x03 failed: connection refused",
	}
	recentLogsMu.Unlock()
	defer func() {
		recentLogsMu.Lock()
		recentLogsRing = saved
		recentLogsMu.Unlock()
	}()

	a := AnalyzeRecentLogs()
	byID := make(map[string]LogEvent)
	for _, ev := range a.Events {
		byID[ev.Pattern] = ev
	}

	if ev, ok := byID["connection-refused"]; !ok {
		t.Error("expected connection-refused detected")
	} else if ev.Count != 3 {
		t.Errorf("count: want 3, got %d", ev.Count)
	}

	if a.TotalErrors < 3 {
		t.Errorf("TotalErrors: want >= 3, got %d", a.TotalErrors)
	}
}

func TestAnalyzeRecentLogs_windowLines(t *testing.T) {
	recentLogsMu.Lock()
	saved := recentLogsRing
	recentLogsRing = []string{"line1", "line2", "line3"}
	recentLogsMu.Unlock()
	defer func() {
		recentLogsMu.Lock()
		recentLogsRing = saved
		recentLogsMu.Unlock()
	}()

	a := AnalyzeRecentLogs()
	if a.WindowLines != 3 {
		t.Errorf("WindowLines: want 3, got %d", a.WindowLines)
	}
}
