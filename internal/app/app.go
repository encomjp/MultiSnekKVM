package app

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"multisnekkvm/internal/audio"
	"multisnekkvm/internal/autostart"
	"multisnekkvm/internal/clipboard"
	"multisnekkvm/internal/discovery"
	"multisnekkvm/internal/filetransfer"
	"multisnekkvm/internal/identity"
	"multisnekkvm/internal/input"
	"multisnekkvm/internal/inputstate"
	"multisnekkvm/internal/logutil"
	"multisnekkvm/internal/protocol"
	"multisnekkvm/internal/resilience"
	"multisnekkvm/internal/settings"
	"multisnekkvm/internal/sysutil"
	"multisnekkvm/internal/tailscale"
	"multisnekkvm/internal/transport"
	"multisnekkvm/internal/trust"
)

type (
	DeviceInfo          = identity.DeviceInfo
	Settings            = settings.Settings
	SettingsStore       = settings.Store
	TrustStore          = trust.Store
	Discovery           = discovery.Discovery
	Transport           = transport.Transport
	InputHook           = input.InputHook
	AudioStreamer       = audio.AudioStreamer
	AudioDevice         = audio.AudioDevice
	TailscaleService    = tailscale.Service
	TailscaleStatus     = tailscale.Status
	FileTransferManager = filetransfer.FileTransferManager
	HealthMonitor       = resilience.HealthMonitor
	HealthStatus        = resilience.HealthStatus
	RemediationAlert    = resilience.RemediationAlert
	PowerWatcher        = sysutil.PowerWatcher
	clipboardSyncState  = clipboard.State
	remoteKeyState      = inputstate.KeyState
	Frame               = protocol.Frame
	EdgeConfigMsg       = protocol.EdgeConfigMsg
	PingMsg             = protocol.PingMsg
	MonitorInfo         = input.MonitorInfo
)

const (
	maxFramePayloadBytes   = protocol.MaxFramePayloadBytes
	maxClipboardTextBytes  = clipboard.MaxClipboardTextBytes
	maxClipboardUTF16Bytes = clipboard.MaxClipboardUTF16Bytes

	MsgPing               = protocol.MsgPing
	MsgPong               = protocol.MsgPong
	MsgEdgeConfig         = protocol.MsgEdgeConfig
	MsgMouseMove          = protocol.MsgMouseMove
	MsgMouseClick         = protocol.MsgMouseClick
	MsgMouseScroll        = protocol.MsgMouseScroll
	MsgKeyDown            = protocol.MsgKeyDown
	MsgKeyUp              = protocol.MsgKeyUp
	MsgClipboard          = protocol.MsgClipboard
	MsgSwitchBack         = protocol.MsgSwitchBack
	MsgAudioStart         = protocol.MsgAudioStart
	MsgAudioStop          = protocol.MsgAudioStop
	MsgAudioData          = protocol.MsgAudioData
	MsgAudioFormat        = protocol.MsgAudioFormat
	MsgMicStart           = protocol.MsgMicStart
	MsgMicStop            = protocol.MsgMicStop
	MsgMicData            = protocol.MsgMicData
	MsgMicFormat          = protocol.MsgMicFormat
	MsgAudioTransport     = protocol.MsgAudioTransport
	MsgMicTransport       = protocol.MsgMicTransport
	MsgFileTransferOffer  = protocol.MsgFileTransferOffer
	MsgFileTransferAccept = protocol.MsgFileTransferAccept
	MsgFileChunk          = protocol.MsgFileChunk
	MsgFileTransferDone   = protocol.MsgFileTransferDone
	MsgFileTransferCancel = protocol.MsgFileTransferCancel
	MsgUnicodeText        = protocol.MsgUnicodeText
)

var remoteKeyIdleTimeout = 2 * time.Second

var (
	NewSettingsStore           = settings.NewStore
	OpenTrustStore             = trust.OpenStore
	LoadOrCreateIdentity       = identity.LoadOrCreateIdentity
	NewInputHook               = input.NewInputHook
	NewAudioStreamer           = audio.NewAudioStreamer
	NewFileTransferManager     = filetransfer.NewFileTransferManager
	NewTransport               = transport.NewTransport
	shortPeerID                = transport.ShortPeerID
	generatePairingCode        = transport.GeneratePairingCode
	NewTailscaleService        = tailscale.NewService
	NewDiscovery               = discovery.NewDiscovery
	NewHealthMonitor           = resilience.NewHealthMonitor
	ListRenderDevices          = audio.ListRenderDevices
	ListCaptureDevices         = audio.ListCaptureDevices
	GetAutostart               = autostart.Get
	SetAutostart               = autostart.Set
	GetRecentLogsSnapshot      = logutil.GetRecentLogsSnapshot
	SafeGo                     = logutil.SafeGo
	SafeGoRestart              = resilience.SafeGoRestart
	DecodeEdgeConfig           = protocol.DecodeEdgeConfig
	DecodeMouseMove            = protocol.DecodeMouseMove
	DecodeMouseClick           = protocol.DecodeMouseClick
	DecodeMouseScroll          = protocol.DecodeMouseScroll
	DecodeKey                  = protocol.DecodeKey
	DecodeClipboard            = protocol.DecodeClipboard
	DecodeClipboardSync        = protocol.DecodeClipboardSync
	DecodeUnicodeText          = protocol.DecodeUnicodeText
	InjectKey                  = input.InjectKey
	InjectUnicode              = input.InjectUnicode
	InjectMouseMove            = input.InjectMouseMove
	InjectMouseClick           = input.InjectMouseClick
	InjectMouseScroll          = input.InjectMouseScroll
	ReleaseAllModifiers        = input.ReleaseAllModifiers
	GetCursorPosition          = input.GetCursorPosition
	GetScreenBounds            = input.GetScreenBounds
	IsSecureDesktopActive      = input.IsSecureDesktopActive
	GetClipboardTextForSync    = input.GetClipboardTextForSync
	GetClipboardSequenceNumber = input.GetClipboardSequenceNumber
	SetClipboardText           = input.SetClipboardText
	WatchPowerEvents           = sysutil.WatchPowerEvents
	EnumLocalMonitors          = input.EnumLocalMonitors
	setClipboardFilesFn        = filetransfer.SetClipboardFiles
)

type PeerInfo struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Address        string            `json:"address"`
	Addresses      []string          `json:"addresses"`
	AddressKinds   map[string]string `json:"addressKinds,omitempty"`
	Fingerprint    string            `json:"fingerprint"`
	Source         string            `json:"source"`
	Routes         []string          `json:"routes"`
	PreferredRoute string            `json:"preferredRoute"`
	Trusted        bool              `json:"trusted"`
	Status         string            `json:"status"`
	LastSeen       int64             `json:"lastSeen"`
}

type SessionStatus struct {
	Route          string `json:"route,omitempty"`
	RemoteAddress  string `json:"remoteAddress,omitempty"`
	Connected      bool   `json:"connected"`
	Controlling    bool   `json:"controlling"`
	PeerName       string `json:"peerName"`
	PeerID         string `json:"peerID"`
	Role           string `json:"role"`
	LatencyMs      int    `json:"latencyMs"`
	AudioLatencyMs int    `json:"audioLatencyMs"`
	JitterMs       int    `json:"jitterMs"`
}

type App struct {
	ctx       context.Context
	cancel    context.CancelFunc
	device    DeviceInfo
	discovery *Discovery
	transport *Transport
	trust     *TrustStore
	inputHook *InputHook
	audio     *AudioStreamer
	tailscale *TailscaleService
	fileTx    *FileTransferManager
	health    *HealthMonitor
	settings  *SettingsStore
	power     *PowerWatcher

	mu                      sync.RWMutex
	manualPeers             map[string]PeerInfo
	controllerEdgeSide      string
	peerControlActive       bool
	peerControlRequiresWake bool
	switchBackSent          bool
	audioMode               string
	audioTiming             string
	audioTransportMode      string
	audioProfile            string
	muteSource              bool
	micMode                 string
	captureDeviceID         string
	playbackDeviceID        string
	micDeviceID             string
	micPlaybackDeviceID     string
	audioOut                [2]*audio.OutboundStream // indexed by audio.StreamKind
	audioIn                 [2]*audio.InboundStream
	audioCapture            [2]*streamCapture
	audioDecodeErrLogged    atomic.Int64
	audioInboundDroppedN    uint64
	sessionRole             string
	clipboardState          clipboardSyncState
	remoteKeyState          remoteKeyState
	remoteMouseButtons      [3]bool    // tracks injected mouse button state (0=left,1=right,2=middle)
	remoteInputMu           sync.Mutex // serializes record+inject vs releaseInjectedRemoteKeys
	remoteKeyTimer          *time.Timer
	remoteKeyDeadline       time.Time
	latencyMs               int
	latencyPrev             int // previous RTT sample for EWMA jitter computation
	jitterMs                int // EWMA of successive RTT deltas; -1 until second sample
	lastPeerAddr            string
	autoReconnect           bool
	reconnecting            bool
	suspended               bool // between OS suspend and resume; suppresses auto-reconnect
	lastConnectTime         time.Time
	tray                    *TrayManager
	quitRequested           bool
	mediaControlMu          sync.Mutex
	mediaControlGeneration  uint64
	pendingRecvDirs         []string // temp dirs awaiting user save/discard; cleaned on shutdown

	// Inbound async dispatch channels — file I/O and audio decode are routed off
	// the readLoop goroutine to prevent TCP backpressure from blocking mouse/key frames.
	fileInboundCh  chan Frame
	audioInboundCh chan Frame

	// Atomic flag: non-zero when remote keyboard keys are currently held.
	// Fast-path in handleRemoteMouseMove to skip touchRemoteKeyWatchdog during
	// pure mouse movement and mouse-button drags.
	remoteInputActiveN uint64

	// Atomic flag: non-zero once remote input has been injected on this
	// machine since the last release. Gates ReleaseAllModifiers so the
	// controller never sends synthetic modifier key-ups.
	injectedInputN uint32

	// UnixNano of the last inbound control-input frame (mouse move, click, scroll,
	// key, or unicode text) received while host is in controlled mode.
	// Set when active peer control starts and on every control frame.
	// Reset to 0 by resetControlledState. Used by controlledModeWatchdog.
	lastRemoteInputNs int64

	// Frontend event deduplication and coalesced session updates.
	events          eventDeduper
	sessionUpdateCh chan struct{}

	// Inbound clipboard writes run off the transport read loop; latest wins.
	clipboardInCh chan string

	// Send mux lanes — see send_mux.go.
	muxHigh  chan Frame
	muxMouse chan Frame
	muxFile  chan Frame
	muxAudio audioLane

	// Outbound mux diagnostic counters (all updated atomically).
	muxHighSentN       uint64
	muxMouseSentN      uint64
	muxFileSentN       uint64
	muxHighDroppedN    uint64
	muxMouseCoalescedN uint64
	muxAudioSentN      uint64
	muxAudioDroppedN   uint64
	muxLastSentNs      int64 // UnixNano of last successful send, 0 = never

	// Rate limiting for send-failure logs (mux goroutine only).
	muxErrLoggedAt   time.Time
	muxErrSuppressed int

	// Inbound receive diagnostic counters (all updated atomically).
	recvMouseMoveN uint64
	recvKeyN       uint64
	recvAudioN     uint64
	recvFileChunkN uint64
	recvOtherN     uint64
}

func NewApp() *App {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "MultiSnekKVM"
	}
	settings := NewSettingsStore()
	cfg := settings.Get()
	return &App{
		device: DeviceInfo{
			Name:        hostname,
			PairingCode: generatePairingCode(),
			Port:        24831,
		},
		settings:            settings,
		sessionUpdateCh:     make(chan struct{}, 1),
		clipboardInCh:       make(chan string, 1),
		manualPeers:         make(map[string]PeerInfo),
		audioMode:           cfg.AudioMode,
		audioTiming:         cfg.AudioTiming,
		audioTransportMode:  cfg.AudioTransport,
		audioProfile:        cfg.AudioProfile,
		muteSource:          cfg.MuteSource,
		micMode:             cfg.MicMode,
		captureDeviceID:     cfg.CaptureDeviceID,
		playbackDeviceID:    cfg.PlaybackDeviceID,
		micDeviceID:         cfg.MicDeviceID,
		micPlaybackDeviceID: cfg.MicPlaybackDeviceID,
		latencyMs:           -1,
		latencyPrev:         -1,
		jitterMs:            -1,
		autoReconnect:       cfg.AutoReconnect == nil || *cfg.AutoReconnect,
	}
}
