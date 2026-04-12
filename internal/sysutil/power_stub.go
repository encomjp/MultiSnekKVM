//go:build !windows

package sysutil

type PowerCallback func(event string)

type PowerWatcher struct {
	done chan struct{}
}

func WatchPowerEvents(cb PowerCallback) *PowerWatcher {
	pw := &PowerWatcher{done: make(chan struct{})}
	close(pw.done)
	return pw
}

func (pw *PowerWatcher) Stop() {}
