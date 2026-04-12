package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"multisnekkvm/internal/sysutil"
)

type Status struct {
	Available    bool     `json:"available"`
	Connected    bool     `json:"connected"`
	BackendState string   `json:"backendState"`
	SelfName     string   `json:"selfName"`
	Tailnet      string   `json:"tailnet"`
	SelfIPs      []string `json:"selfIPs"`
	PeerCount    int      `json:"peerCount"`
	TargetCount  int      `json:"targetCount"`
	LastSync     int64    `json:"lastSync"`
	LastError    string   `json:"lastError"`
}

type Service struct {
	mu         sync.RWMutex
	executable string
	status     Status
	targetIPs  []string
}

type tailscaleStatusResponse struct {
	BackendState   string                         `json:"BackendState"`
	MagicDNSSuffix string                         `json:"MagicDNSSuffix"`
	TailscaleIPs   []string                       `json:"TailscaleIPs"`
	Self           *tailscaleStatusPeer           `json:"Self"`
	Peer           map[string]tailscaleStatusPeer `json:"Peer"`
}

type tailscaleStatusPeer struct {
	ID           string   `json:"ID"`
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

func NewService() *Service {
	executable := findTailscaleExecutable()
	return &Service{
		executable: executable,
		status: Status{
			Available: executable != "",
		},
	}
}

func (service *Service) Run(ctx context.Context) {
	service.refresh(ctx)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.refresh(ctx)
		}
	}
}

func (service *Service) Status() Status {
	service.mu.RLock()
	defer service.mu.RUnlock()
	status := service.status
	status.SelfIPs = append([]string(nil), service.status.SelfIPs...)
	return status
}

func (service *Service) TargetIPs() []string {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return append([]string(nil), service.targetIPs...)
}

func (service *Service) refresh(ctx context.Context) {
	status := Status{Available: service.executable != ""}
	if service.executable == "" {
		service.mu.Lock()
		service.status = status
		service.targetIPs = nil
		service.mu.Unlock()
		return
	}

	refreshCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(refreshCtx, service.executable, "status", "--json")
	sysutil.HideCommandWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		status.LastError = tailscaleErrorMessage(err)
		status.LastSync = time.Now().Unix()
		service.mu.Lock()
		service.status = status
		service.targetIPs = nil
		service.mu.Unlock()
		return
	}

	var response tailscaleStatusResponse
	if err := json.Unmarshal(output, &response); err != nil {
		status.LastError = err.Error()
		status.LastSync = time.Now().Unix()
		service.mu.Lock()
		service.status = status
		service.targetIPs = nil
		service.mu.Unlock()
		return
	}

	selfIPs := uniqueStrings(response.TailscaleIPs)
	if len(selfIPs) == 0 && response.Self != nil {
		selfIPs = uniqueStrings(response.Self.TailscaleIPs)
	}
	status.BackendState = response.BackendState
	status.Connected = response.BackendState == "Running" && len(selfIPs) > 0
	status.SelfIPs = selfIPs
	status.SelfName = firstNonEmptyString(peerDisplayName(response.Self), os.Getenv("COMPUTERNAME"))
	status.Tailnet = strings.Trim(strings.TrimSpace(response.MagicDNSSuffix), ".")
	status.LastSync = time.Now().Unix()

	targets := make([]string, 0, len(response.Peer))
	for _, peer := range response.Peer {
		ips := uniqueStrings(peer.TailscaleIPs)
		if len(ips) == 0 {
			continue
		}
		status.PeerCount++
		targets = append(targets, ips...)
	}

	targets = uniqueStrings(targets)
	sort.Strings(targets)
	status.TargetCount = len(targets)

	service.mu.Lock()
	service.status = status
	service.targetIPs = targets
	service.mu.Unlock()
}

func findTailscaleExecutable() string {
	if executable, err := exec.LookPath("tailscale"); err == nil {
		return executable
	}

	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Tailscale", "tailscale.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Tailscale", "tailscale.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Tailscale", "tailscale.exe"),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

func tailscaleErrorMessage(err error) string {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		message := strings.TrimSpace(string(exitError.Stderr))
		if message != "" {
			return message
		}
	}
	return err.Error()
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func peerDisplayName(peer *tailscaleStatusPeer) string {
	if peer == nil {
		return ""
	}
	return firstNonEmptyString(strings.TrimSuffix(peer.DNSName, "."), peer.HostName, peer.ID)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
