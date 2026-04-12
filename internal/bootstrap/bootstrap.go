package bootstrap

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	appcore "multisnekkvm/internal/app"
	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/sysutil"
)

const supervisedEnvKey = "MULTISNEK_SUPERVISED"

func Run(assets fs.FS, trayIcon, appIcon []byte) {
	if runMode(os.Getenv(supervisedEnvKey)) == "supervisor" {
		runSupervisor(os.Args, os.Environ())
		return
	}

	runChild(assets, trayIcon, appIcon)
}

func runMode(value string) string {
	if value == "1" {
		return "child"
	}
	return "supervisor"
}

func supervisedEnv(base []string) []string {
	prefix := supervisedEnvKey + "="
	out := make([]string, 0, len(base)+1)
	replaced := false
	for _, entry := range base {
		if strings.HasPrefix(entry, prefix) {
			out = append(out, prefix+"1")
			replaced = true
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, prefix+"1")
	}
	return out
}

func runSupervisor(args, env []string) {
	const maxRestarts = 10
	const cooldown = 3 * time.Second

	fmt.Fprintf(os.Stderr, "bootstrap.supervisor.start argv=%q max_restarts=%d cooldown=%s\n", strings.Join(args, " "), maxRestarts, cooldown)
	for attempt := 1; attempt <= maxRestarts; attempt++ {
		cmd := exec.Command(args[0], args[1:]...)
		sysutil.HideCommandWindow(cmd)
		cmd.Env = supervisedEnv(env)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		fmt.Fprintf(os.Stderr, "bootstrap.supervisor.spawn attempt=%d/%d cmd=%q\n", attempt, maxRestarts, strings.Join(cmd.Args, " "))
		err := cmd.Run()
		if err == nil {
			fmt.Fprintf(os.Stderr, "bootstrap.supervisor.stop reason=clean_exit attempt=%d\n", attempt)
			return
		}

		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bootstrap.supervisor.restart attempt=%d/%d exit_code=%d err=%q cooldown=%s\n", attempt, maxRestarts, exitCode, err.Error(), cooldown)
		time.Sleep(cooldown)
	}

	fmt.Fprintf(os.Stderr, "bootstrap.supervisor.abort reason=max_restarts_reached max_restarts=%d\n", maxRestarts)
	os.Exit(1)
}

func runChild(assets fs.FS, trayIcon, appIcon []byte) {
	logutil.InitLogger()
	defer logutil.CloseLogger()

	logutil.LogKV("bootstrap.child.start",
		"pid", os.Getpid(),
		"argv", strings.Join(os.Args, " "),
		"tray_icon_bytes", len(trayIcon),
		"app_icon_bytes", len(appIcon),
	)

	if err := runApp(assets, trayIcon, appIcon); err != nil {
		logutil.LogKV("bootstrap.child.exit", "status", "error", "error", err)
		os.Exit(1)
	}

	logutil.LogKV("bootstrap.child.exit", "status", "clean")
}

func runApp(assets fs.FS, trayIcon, appIcon []byte) error {
	appcore.SetEmbeddedIcons(trayIcon, appIcon)
	app := appcore.NewApp()
	logutil.LogKV("bootstrap.app.configure",
		"title", "MultiSnekKVM",
		"width", 1360,
		"height", 860,
		"min_width", 1100,
		"min_height", 720,
	)
	return wails.Run(buildOptions(assets, app))
}

func buildOptions(assets fs.FS, app *appcore.App) *options.App {
	return &options.App{
		Title:            "MultiSnekKVM",
		Width:            1360,
		Height:           860,
		MinWidth:         1100,
		MinHeight:        720,
		BackgroundColour: &options.RGBA{R: 9, G: 9, B: 11, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:     app.Startup,
		OnShutdown:    app.Shutdown,
		OnBeforeClose: app.BeforeClose,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
	}
}