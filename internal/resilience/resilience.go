package resilience

import (
	"context"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"multisnekkvm/internal/logutil"
)

const (
	minRestartDelay = 500 * time.Millisecond
	maxRestartDelay = 30 * time.Second
	healthCheckFreq = 5 * time.Second
	healthGracePeriod = 15 * time.Second
)

// SafeGoRestart launches fn in a goroutine that auto-recovers from panics.
func SafeGoRestart(ctx context.Context, name string, fn func(ctx context.Context)) {
	go func() {
		delay := minRestartDelay
		for {
			if ctx.Err() != nil {
				return
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						buf := make([]byte, 64*1024)
						n := runtime.Stack(buf, false)
						log.Printf("PANIC in %q: %v\n%s", name, r, buf[:n])
						logutil.WriteCrashDump(name, r, buf[:n])
					}
				}()
				fn(ctx)
			}()
			if ctx.Err() != nil {
				return
			}
			log.Printf("goroutine %q exited, restarting in %v", name, delay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay = delay * 2
			if delay > maxRestartDelay {
				delay = maxRestartDelay
			}
		}
	}()
}

// SubsystemStatus represents the health of one subsystem.
type SubsystemStatus struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail"`
}

// RemediationAlert is emitted when a subsystem triggers auto-remediation.
type RemediationAlert struct {
	Subsystem string `json:"subsystem"`
	Message   string `json:"message"`
}

// HealthStatus is the snapshot returned to the frontend.
type HealthStatus struct {
	Healthy        bool              `json:"healthy"`
	Reconnecting   bool              `json:"reconnecting"`
	Subsystems     []SubsystemStatus `json:"subsystems"`
	Uptime         int               `json:"uptime"`
	Goroutines     int               `json:"goroutines"`
	GoroutineDelta int               `json:"goroutineDelta"`
}

// HealthMonitor watches subsystems and exposes aggregated health.
type HealthMonitor struct {
	mu     sync.RWMutex
	checks []healthCheck
	start  time.Time

	reconnecting atomic.Bool

	// Goroutine leak detection: tracks baseline goroutine count and
	// alerts when the count grows significantly beyond it.
	goroutineBaseline atomic.Int64
	goroutineLeakMu   sync.Mutex
	goroutineLeakHigh int // high-water mark for goroutine count
}

type healthCheck struct {
	name           string
	fn             func() (healthy bool, detail string)
	remediateFn    func() // called after remediateAfter consecutive failures; nil disables
	remediateAfter int    // 0 = no remediation
}

func NewHealthMonitor() *HealthMonitor {
	return &HealthMonitor{start: time.Now()}
}

func (h *HealthMonitor) Register(name string, fn func() (bool, string)) {
	h.mu.Lock()
	h.checks = append(h.checks, healthCheck{name: name, fn: fn})
	h.mu.Unlock()
}

// RegisterWithRemediation is like Register but triggers remediate() after
// the check fails for after consecutive polls. remediate runs in a new goroutine
// so it must not call Status() (would deadlock on h.mu) or block the health loop.
func (h *HealthMonitor) RegisterWithRemediation(name string, fn func() (bool, string), after int, remediate func()) {
	h.mu.Lock()
	h.checks = append(h.checks, healthCheck{
		name:           name,
		fn:             fn,
		remediateFn:    remediate,
		remediateAfter: after,
	})
	h.mu.Unlock()
}

func (h *HealthMonitor) SetReconnecting(v bool) {
	h.reconnecting.Store(v)
}

// runPoll executes all registered health checks and returns the status snapshot
// together with the check slice that was used to produce it. Both are consistent:
// they derive from the same h.mu-protected snapshot so callers can correlate
// Subsystems[i] ↔ checks[i] without a second lock.
func (h *HealthMonitor) runPoll() (HealthStatus, []healthCheck) {
	h.mu.RLock()
	checks := append([]healthCheck(nil), h.checks...)
	h.mu.RUnlock()

	uptime := time.Since(h.start)
	inGrace := uptime < healthGracePeriod

	allOK := true
	subs := make([]SubsystemStatus, 0, len(checks))
	for _, c := range checks {
		ok, detail := c.fn()
		if inGrace {
			ok = true
		}
		if !ok {
			allOK = false
		}
		subs = append(subs, SubsystemStatus{Name: c.name, Healthy: ok, Detail: detail})
	}

	numGoroutines := runtime.NumGoroutine()
	baseline := int(h.goroutineBaseline.Load())
	if baseline == 0 {
		h.goroutineBaseline.CompareAndSwap(0, int64(numGoroutines))
		baseline = int(h.goroutineBaseline.Load())
	}
	delta := numGoroutines - baseline

	h.goroutineLeakMu.Lock()
	if numGoroutines > h.goroutineLeakHigh {
		h.goroutineLeakHigh = numGoroutines
	}
	h.goroutineLeakMu.Unlock()

	if delta > 50 && !inGrace {
		log.Printf("health: goroutine count elevated (current=%d baseline=%d delta=+%d)", numGoroutines, baseline, delta)
	}

	return HealthStatus{
		Healthy:        allOK,
		Reconnecting:   h.reconnecting.Load(),
		Subsystems:     subs,
		Uptime:         int(uptime.Seconds()),
		Goroutines:     numGoroutines,
		GoroutineDelta: delta,
	}, checks
}

// Status returns a point-in-time health snapshot. It is safe to call from any
// goroutine and has no side effects (no failure counters are modified here).
func (h *HealthMonitor) Status() HealthStatus {
	s, _ := h.runPoll()
	return s
}

// Run polls health every healthCheckFreq, emits snapshots via emit, and triggers
// registered remediation functions when a subsystem fails consecutively.
func (h *HealthMonitor) Run(ctx context.Context, emit func(HealthStatus)) {
	ticker := time.NewTicker(healthCheckFreq)
	defer ticker.Stop()

	// failStreak tracks consecutive failure counts per check name.
	// Using a map by name (not index) is safe against checks being added at runtime.
	failStreak := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status, checks := h.runPoll()

			// Apply remediation logic using the same check snapshot used for status.
			for i, c := range checks {
				if c.remediateFn == nil || c.remediateAfter <= 0 {
					continue
				}
				if i >= len(status.Subsystems) {
					break
				}
				if status.Subsystems[i].Healthy {
					failStreak[c.name] = 0
					continue
				}
				failStreak[c.name]++
				if failStreak[c.name] == c.remediateAfter {
					log.Printf("health: subsystem %q failed %d consecutive checks — triggering remediation", c.name, failStreak[c.name])
					logutil.SafeGo("remediate-"+c.name, c.remediateFn)
				}
			}

			emit(status)
		}
	}
}
