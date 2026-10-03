package app

import (
	"reflect"
	"sync"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// eventDeduper suppresses frontend events whose payload is identical to the
// previous emission of the same event. All emitters of a deduplicated event
// must go through it so "last emitted" reflects what the frontend has.
type eventDeduper struct {
	mu   sync.Mutex
	last map[string]any
}

// emitIfChanged emits name with payload unless it equals the last payload
// emitted for name. The lock is held across the emit so the recorded value
// matches the order the frontend receives events in.
func (a *App) emitIfChanged(name string, payload any) bool {
	if a.ctx == nil {
		return false
	}
	d := &a.events
	d.mu.Lock()
	defer d.mu.Unlock()
	if prev, ok := d.last[name]; ok && reflect.DeepEqual(prev, payload) {
		return false
	}
	if d.last == nil {
		d.last = make(map[string]any)
	}
	d.last[name] = payload
	wailsRuntime.EventsEmit(a.ctx, name, payload)
	return true
}

func (a *App) emitSessionUpdated() SessionStatus {
	sess := a.GetSession()
	a.emitIfChanged("session-updated", sess)
	return sess
}

func (a *App) emitPeersUpdated() {
	a.emitIfChanged("peers-updated", a.GetPeers())
}

// requestSessionUpdate asks the session-update worker to emit a fresh
// session status. It never blocks, so it is safe on the transport read
// loop; bursts of requests coalesce into a single emission.
func (a *App) requestSessionUpdate() {
	select {
	case a.sessionUpdateCh <- struct{}{}:
	default:
	}
}

func (a *App) sessionUpdateLoop() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.sessionUpdateCh:
			a.emitSessionUpdated()
		}
	}
}
