package logutil

import (
	"strings"
	"time"
)

// LogEvent describes a detected anomaly pattern in recent in-memory logs.
type LogEvent struct {
	Level   string `json:"level"`   // "critical", "error", "warn", "info"
	Pattern string `json:"pattern"` // short identifier
	Count   int    `json:"count"`   // occurrences in the analysis window
	Sample  string `json:"sample"`  // first matching log line
}

// LogAnalysis is the result of AnalyzeRecentLogs.
type LogAnalysis struct {
	Events      []LogEvent `json:"events"`
	TotalErrors int        `json:"totalErrors"`
	WindowLines int        `json:"windowLines"` // number of log lines analyzed
	AnalyzedAt  string     `json:"analyzedAt"`
}

type logPattern struct {
	id       string
	level    string
	keywords []string // all must be present in the line (case-insensitive)
}

// logPatterns in priority order — critical first, info last.
var patterns = []logPattern{
	{id: "panic", level: "critical", keywords: []string{"panic"}},
	{id: "send-stall", level: "error", keywords: []string{"send-mux stall"}},
	{id: "send-stall-critical", level: "critical", keywords: []string{"stall critical", "forcing session disconnect"}},
	{id: "transport-error", level: "error", keywords: []string{"transport error"}},
	{id: "io-timeout", level: "error", keywords: []string{"i/o timeout"}},
	{id: "connection-refused", level: "error", keywords: []string{"connection refused"}},
	{id: "eof", level: "error", keywords: []string{"unexpected eof"}},
	{id: "broken-pipe", level: "error", keywords: []string{"broken pipe"}},
	{id: "send-frame-failed", level: "error", keywords: []string{"send frame", "failed"}},
	{id: "queue-overflow", level: "warn", keywords: []string{"high lane full"}},
	{id: "slow-write", level: "warn", keywords: []string{"slow write"}},
	{id: "goroutine-leak", level: "warn", keywords: []string{"goroutine count elevated"}},
	{id: "health-remediation", level: "warn", keywords: []string{"health-remediation"}},
	{id: "reconnect-attempt", level: "info", keywords: []string{"auto-reconnect: attempt"}},
	{id: "reconnect-success", level: "info", keywords: []string{"auto-reconnect: success"}},
	{id: "controlled-release", level: "info", keywords: []string{"auto-releasing controlled state"}},
	{id: "power-suspend", level: "info", keywords: []string{"system suspending"}},
}

// AnalyzeRecentLogs scans the in-memory log ring and classifies entries by pattern.
// Lines tagged [log-watchdog] are excluded to prevent self-referential alerts.
func AnalyzeRecentLogs() LogAnalysis {
	lines := GetRecentLogsSnapshot()

	type match struct {
		count  int
		sample string
	}
	hits := make(map[string]*match, len(patterns))

	totalErrors := 0
	for _, line := range lines {
		lower := strings.ToLower(line)
		// Skip watchdog-emitted summary lines to avoid re-detecting their content.
		if strings.Contains(lower, "[log-watchdog]") {
			continue
		}
		for _, p := range patterns {
			ok := true
			for _, kw := range p.keywords {
				if !strings.Contains(lower, kw) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			if _, exists := hits[p.id]; !exists {
				hits[p.id] = &match{}
			}
			m := hits[p.id]
			m.count++
			if m.sample == "" {
				m.sample = line
			}
			if p.level == "error" || p.level == "critical" {
				totalErrors++
			}
		}
	}

	// Build events preserving the declaration order for stable output.
	events := make([]LogEvent, 0, len(hits))
	for _, p := range patterns {
		m, ok := hits[p.id]
		if !ok {
			continue
		}
		events = append(events, LogEvent{
			Level:   p.level,
			Pattern: p.id,
			Count:   m.count,
			Sample:  m.sample,
		})
	}

	return LogAnalysis{
		Events:      events,
		TotalErrors: totalErrors,
		WindowLines: len(lines),
		AnalyzedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}
