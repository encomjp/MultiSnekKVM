package app

import "context"

// LifecycleHooks returns the Wails lifecycle callbacks for a.
//
// Wails binds every exported method of a bound struct to JavaScript, so the
// lifecycle methods are unexported and handed to the runtime through this
// package-level function instead (functions are never bound).
func LifecycleHooks(a *App) (
	startup func(context.Context),
	shutdown func(context.Context),
	beforeClose func(context.Context) bool,
) {
	return a.startup, a.shutdown, a.beforeClose
}
