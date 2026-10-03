<div align="center">

# 🐍 MultiSnek KVM

**A desktop KVM switch for trusted peers across your LAN and tailnet.**

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Wails](https://img.shields.io/badge/Wails-v2-C92A2A?logo=webassembly&logoColor=white)](https://wails.io)
[![Svelte](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte&logoColor=white)](https://svelte.dev)
[![Platform](https://img.shields.io/badge/Platform-Windows-0078D6?logo=windows&logoColor=white)]()
[![Release](https://img.shields.io/github/v/release/encomjp/MultiSnekKVM?logo=github&label=Release)](https://github.com/encomjp/MultiSnekKVM/releases/latest)
[![License](https://img.shields.io/github/license/encomjp/MultiSnekKVM?color=blue)](LICENSE)

Move your mouse to the screen edge → cursor jumps to the other PC.  
Keyboard, clipboard, audio, and mic follow seamlessly.

<br>

[![Download Latest Release](https://img.shields.io/badge/⬇_Download_Latest_Release-238636?style=for-the-badge&logo=github&logoColor=white)](https://github.com/encomjp/MultiSnekKVM/releases/latest)

[View Changelog](CHANGELOG.md) · [Latest Release Notes](https://github.com/encomjp/MultiSnekKVM/releases/latest)

</div>

---

## 🖼️ The app

A sidebar keeps three screens one click away, with **This PC** (listening status and the pairing PIN, hidden until you ask) always at the bottom. Dark and light themes follow Windows or your choice.

| Screen | What you get |
|--------|--------------|
| **Session** | Who you are controlling, the route in use (USB4, Ethernet, Wi-Fi, Tailnet, ...), latency, jitter and audio at a glance; a sketch of your screen arrangement; an activity feed where received files wait for **Save to Downloads** or **Discard**; quick audio, microphone and auto-reconnect controls; live subsystem health and alerts. |
| **Devices** | Every known PC with its online and pairing state. PCs reachable over several routes list them as radio buttons with the best one marked, so you can pick the route for the next connection. New PCs are paired once with their six-digit PIN (**Pair & connect**). A side panel shows this PC's adapters and how to link over USB4 or Bluetooth. |
| **Settings** | **Input & screens** (hand-off edge, monitor layout with a draggable hand-off zone and return point, pointer speed, exit hotkey recorder), **Audio** (direction, devices, Auto/PCM/Opus transport, microphone), **Startup** (autostart, tray, theme), **Security** (certificate fingerprint, paired devices, Tailscale) and **Diagnostics** (health, load counters, log analysis, copyable recent logs). |

---

## 📈 Performance (real measured values)

<table>
<tr>
<td align="center"><strong>~45 MB</strong><br><sub>RAM usage</sub></td>
<td align="center"><strong>~4% CPU</strong><br><sub>avg with active session</sub></td>
<td align="center"><strong>~7 KB/s</strong><br><sub>network w/ audio streaming</sub></td>
<td align="center"><strong>30</strong><br><sub>goroutines</sub></td>
</tr>
</table>

> Measured on Windows 11 with one peer connected and desktop audio streaming active. No Electron, no browser — just a native Go binary with a thin webview.

---

## ✨ Features

| | Feature | Description |
|---|---------|-------------|
| 🖱️ | **Seamless Mouse & Keyboard** | Edge-triggered cursor handoff with configurable four-edge detection |
| 📋 | **Clipboard Sync** | Copy on one PC, paste on the other — instant, bidirectional |
| 🔊 | **Desktop Audio Streaming** | Hear the remote PC's audio or broadcast yours (WASAPI loopback) |
| 🎤 | **Microphone Forwarding** | Send your mic to the remote PC or hear theirs |
| 🔒 | **Authenticated Peer Trust** | TLS 1.3, certificate-bound SPAKE2 PIN pairing, fingerprint-pinned reconnects |
| 🌐 | **LAN + Tailscale Discovery** | Auto-discovers peers via UDP broadcast and Tailscale status |
| 🔄 | **Auto-Reconnect** | Exponential backoff reconnection on unexpected disconnect |
| 💓 | **Health Monitor** | Real-time subsystem health checks and alerts on the Session screen and in Settings → Diagnostics |
| 📊 | **Latency Display** | Live ping/pong RTT and jitter between peers |
| 🛡️ | **Process Watchdog** | Supervisor/child pattern auto-restarts on crash |
| 🧵 | **Goroutine Recovery** | `SafeGoRestart` wraps background goroutines with panic recovery |
| 🖥️ | **System Tray** | Minimize to tray, quick access controls |
| 📂 | **File Transfer** | Send files with the Send files button; received files wait in Session → Activity until you save or discard them |

### ⚠️ Known Issues

| Issue | Status |
|-------|--------|
| **Drag-and-drop across edge** | Disabled — dragging a file to the screen edge does not reliably transfer it to the second monitor due to OLE/multi-monitor conflicts. Use the **Send Files** button instead. In progress. |

## 🚀 Quick Start

### Dev Mode

```powershell
wails dev
```

### Production Build

```powershell
.\build.ps1
```

Output: `build/bin/Multisnek.exe`

> **Note:** `build.ps1` requires [MSYS2 MinGW](https://www.msys2.org/) at `C:\msys64\mingw64\bin` and static Opus/Ogg libraries. CGO is enabled automatically.

## 🔐 Trust Model

| Scenario | Behavior |
|----------|----------|
| **First contact** | Enter the six-digit PIN shown on the remote device. A certificate-bound SPAKE2 exchange verifies both peers before their certificates are pinned; the PIN itself is never sent on the wire |
| **Known peer** | Future sessions must present the same fingerprint or the connection is rejected |
| **Identity change** | Regenerated certificates or renamed identities require re-trust before traffic is accepted |

Connections run over TLS 1.3 with device certificates and fingerprint pinning. Existing trusted peers reconnect without the PIN. For new pairing, **both devices must run a version supporting SPAKE2**; insecure legacy plaintext-PIN pairing is rejected. The current SPAKE2 dependency has not yet received an independent security audit, so security-sensitive deployments should review the pairing implementation before release.

## 🔌 Direct USB4, Thunderbolt and Bluetooth connections

MultiSnek's existing TCP/TLS transport can use **any functioning IP network adapter**. The app discovers peer addresses on usable adapters, prefers USB4/Thunderbolt networking and supported network-class USB bridges, then other wired and wireless routes, and retries alternate addresses after network connection failures. It does not switch the transport under an active KVM session. A manually selected address is tried first. Authentication failures never cause an automatic route fallback. It does not need a separate raw USB or Bluetooth protocol. The **Devices** screen lists this PC's adapters (with **Connect over USB4 or Bluetooth** guidance) and, for each peer, its available routes as radio buttons with the best one marked.

- **USB4 / Thunderbolt:** On compatible Windows 11 PCs, connect with a suitable USB4/Thunderbolt cable. Windows can create an Ethernet-over-USB4 (USB4NET) adapter with an automatically assigned `169.254.x.x` address. Find each PC's address in the Devices adapters panel or with `ipconfig`, and enter **the other PC's** address in MultiSnek. Pick the direct-link route in the peer's route list when both LAN/Tailscale and the direct link are present.
- **Bluetooth PAN:** Pair the computers. If the host supports a Bluetooth hotspot/PAN service, enable it and join the PAN in Windows' Bluetooth device settings. Once each device has an IP-capable Bluetooth network connection, enter the peer's PAN address manually. This is a fallback for input/clipboard; Bluetooth bandwidth and latency can limit audio and file transfers.
- **Other wired links:** Ordinary Ethernet or dedicated USB transfer/bridge hardware **only if its driver exposes a working IP adapter** can use the same manual-address workflow.

An ordinary USB-C charging/data cable between two standard USB hosts **does not** create a PC-to-PC network. USB4/Thunderbolt interdomain networking requires compatible hardware, OS support and a suitable cable. A file-transfer-only USB bridge cable is not automatically compatible, and raw WinUSB/libusb endpoints are not enabled without a supported bridge controller and its documented framing/driver API.

**Troubleshooting:** Confirm the adapter is up on both PCs, test reachability to the peer's IP, allow MultiSnek's TCP port `24831` through Windows Firewall on the new network, and use the actual peer IP rather than the hostname if Windows selects Wi-Fi instead of the direct link. Standard LAN discovery uses IPv4 broadcasts; if discovery is blocked, add the peer IP manually. This feature does not install network drivers or automatically establish a Bluetooth PAN.

References: [Microsoft USB4 interdomain networking](https://learn.microsoft.com/en-us/windows-hardware/design/component-guidelines/usb4-interdomain-connections) · [Microsoft Bluetooth PAN setup](https://support.microsoft.com/en-us/windows/hardware/bluetooth/connect-to-a-bluetooth-network-in-windows)

## ⚙️ Requirements

- **Windows 10/11** (primary platform)
- **Go 1.25+**
- **Node.js 18+**
- **Wails v2** — `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **MSYS2 MinGW** (for production builds with Opus codec)

## 📜 License

See [LICENSE](LICENSE) for details.

