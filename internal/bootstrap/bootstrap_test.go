package bootstrap

import (
	"reflect"
	"testing"

	appcore "multisnekkvm/internal/app"
)

func TestRunMode(t *testing.T) {
	if got := runMode(""); got != "supervisor" {
		t.Fatalf("expected supervisor mode, got %q", got)
	}
	if got := runMode("1"); got != "child" {
		t.Fatalf("expected child mode, got %q", got)
	}
}

func TestSupervisedEnvAddsFlag(t *testing.T) {
	env := supervisedEnv([]string{"PATH=test"})
	if len(env) != 2 {
		t.Fatalf("expected 2 env entries, got %d", len(env))
	}
	if env[1] != supervisedEnvKey+"=1" {
		t.Fatalf("expected supervised flag to be appended, got %q", env[1])
	}
}

func TestSupervisedEnvReplacesExistingFlag(t *testing.T) {
	env := supervisedEnv([]string{supervisedEnvKey + "=0", "PATH=test"})
	if env[0] != supervisedEnvKey+"=1" {
		t.Fatalf("expected supervised flag to be replaced, got %q", env[0])
	}
	if env[1] != "PATH=test" {
		t.Fatalf("expected other env vars to stay in place, got %q", env[1])
	}
}

func TestLifecycleMethodsAreNotBoundToJS(t *testing.T) {
	appType := reflect.TypeOf(&appcore.App{})
	for _, name := range []string{"Startup", "Shutdown", "BeforeClose"} {
		if _, ok := appType.MethodByName(name); ok {
			t.Errorf("*app.App exports %s; Wails would expose it to JavaScript", name)
		}
	}
	opts := buildOptions(nil, &appcore.App{})
	if opts.OnStartup == nil || opts.OnShutdown == nil || opts.OnBeforeClose == nil {
		t.Fatal("lifecycle hooks must still be wired")
	}
}
