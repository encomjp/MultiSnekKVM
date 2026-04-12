package logutil

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var (
	recentLogsMu   sync.Mutex
	recentLogsRing []string
)

const recentLogsMax = 150

func appendRecentLog(line string) {
	recentLogsMu.Lock()
	defer recentLogsMu.Unlock()
	recentLogsRing = append(recentLogsRing, line)
	if len(recentLogsRing) > recentLogsMax {
		recentLogsRing = recentLogsRing[len(recentLogsRing)-recentLogsMax:]
	}
}

func GetRecentLogsSnapshot() []string {
	recentLogsMu.Lock()
	defer recentLogsMu.Unlock()
	out := make([]string, len(recentLogsRing))
	copy(out, recentLogsRing)
	return out
}

var logFile *os.File

// InitLogger sets up file-based logging to %APPDATA%/MultiSnekKVM/logs/.
func InitLogger() {
	dir, err := dataDir()
	if err != nil {
		log.Printf("logger: cannot determine data dir: %v", err)
		return
	}
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.Printf("logger: cannot create log dir: %v", err)
		return
	}

	pruneOldLogs(logDir, 5)

	name := fmt.Sprintf("multisnek_%s.log", time.Now().Format("2006-01-02_15-04-05"))
	path := filepath.Join(logDir, name)
	f, err := os.Create(path)
	if err != nil {
		log.Printf("logger: cannot create log file: %v", err)
		return
	}
	logFile = f

	log.SetOutput(&dualWriter{file: f, stderr: os.Stderr})
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Printf("─── Multisnek session started ── log: %s ───", path)
}

// CloseLogger flushes and closes the log file.
func CloseLogger() {
	if logFile != nil {
		log.Printf("─── Multisnek session ended ───")
		logFile.Sync()
		logFile.Close()
	}
}

type dualWriter struct {
	file   *os.File
	stderr *os.File
}

func (w *dualWriter) Write(p []byte) (int, error) {
	w.stderr.Write(p)
	for _, seg := range strings.Split(string(p), "\n") {
		if seg != "" {
			appendRecentLog(seg)
		}
	}
	return w.file.Write(p)
}

// LogKV emits a structured key=value log line while preserving the standard logger output.
func LogKV(event string, keyvals ...any) {
	if strings.TrimSpace(event) == "" {
		event = "event"
	}
	if len(keyvals)%2 != 0 {
		keyvals = append(keyvals, "<missing>")
	}

	var builder strings.Builder
	builder.WriteString(event)
	for index := 0; index < len(keyvals); index += 2 {
		key := strings.TrimSpace(fmt.Sprint(keyvals[index]))
		if key == "" {
			key = fmt.Sprintf("arg%d", index/2)
		}
		builder.WriteByte(' ')
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(formatValue(keyvals[index+1]))
	}
	log.Print(builder.String())
}

func formatValue(value any) string {
	switch typed := value.(type) {
	case error:
		return fmt.Sprintf("%q", typed.Error())
	case fmt.Stringer:
		return fmt.Sprintf("%q", typed.String())
	case string:
		return fmt.Sprintf("%q", typed)
	default:
		return fmt.Sprint(value)
	}
}

// SafeGo launches a goroutine with panic recovery that logs the full stack trace.
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 64*1024)
				n := runtime.Stack(buf, false)
				log.Printf("PANIC in goroutine %q: %v\n%s", name, r, buf[:n])
				WriteCrashDump(name, r, buf[:n])
			}
		}()
		fn()
	}()
}

// WriteCrashDump writes a crash report to a dedicated file.
func WriteCrashDump(goroutineName string, panicValue interface{}, stack []byte) {
	dir, err := dataDir()
	if err != nil {
		return
	}
	logDir := filepath.Join(dir, "logs")
	os.MkdirAll(logDir, 0o755)

	name := fmt.Sprintf("crash_%s.log", time.Now().Format("2006-01-02_15-04-05"))
	path := filepath.Join(logDir, name)
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "═══ MULTISNEK CRASH REPORT ═══\n")
	fmt.Fprintf(f, "Time:      %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "Goroutine: %s\n", goroutineName)
	fmt.Fprintf(f, "Panic:     %v\n", panicValue)
	fmt.Fprintf(f, "\n─── Stack Trace ───\n%s\n", stack)

	allBuf := make([]byte, 256*1024)
	n := runtime.Stack(allBuf, true)
	fmt.Fprintf(f, "\n─── All Goroutines ───\n%s\n", allBuf[:n])

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(f, "\n─── Memory ───\n")
	fmt.Fprintf(f, "HeapAlloc:  %d MB\n", m.HeapAlloc/1024/1024)
	fmt.Fprintf(f, "HeapSys:    %d MB\n", m.HeapSys/1024/1024)
	fmt.Fprintf(f, "HeapInuse:  %d MB\n", m.HeapInuse/1024/1024)
	fmt.Fprintf(f, "NumGC:      %d\n", m.NumGC)
	fmt.Fprintf(f, "Goroutines: %d\n", runtime.NumGoroutine())

	bi, ok := debug.ReadBuildInfo()
	if ok {
		fmt.Fprintf(f, "\n─── Build Info ───\n%s\n", bi)
	}
}

func pruneOldLogs(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var logs []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".log" {
			logs = append(logs, e)
		}
	}
	if len(logs) <= keep {
		return
	}
	for i := 0; i < len(logs)-keep; i++ {
		os.Remove(filepath.Join(dir, logs[i].Name()))
	}
}

// LogMemStats logs current memory statistics.
func LogMemStats(label string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("[mem:%s] heap=%dMB sys=%dMB inuse=%dMB gc=%d goroutines=%d",
		label, m.HeapAlloc/1024/1024, m.HeapSys/1024/1024,
		m.HeapInuse/1024/1024, m.NumGC, runtime.NumGoroutine())
}

func appDataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "MultiSnekKVM")
	return path, os.MkdirAll(path, 0o700)
}

func dataDir() (string, error) {
	return appDataDir()
}
