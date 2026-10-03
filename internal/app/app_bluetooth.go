package app

import (
	"context"
	"log"
	"net"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"multisnekkvm/internal/bluetooth"
	"multisnekkvm/internal/discovery"
	"multisnekkvm/internal/settings"
)

// initBluetooth wires the native Bluetooth link: it advertises this PC to
// paired devices, accepts their sessions through the normal transport (same
// TLS, pairing and trust rules as TCP), and feeds the PCs it finds into the
// peer list. Without a radio it stays idle and reports why.
func (a *App) initBluetooth() {
	enabled := true
	if v := a.settings.Get().BluetoothEnabled; v != nil {
		enabled = *v
	}
	m := bluetooth.NewManager(bluetooth.NewBackend(), a.device.ID, a.device.Name, enabled)
	a.bluetooth = m
	a.transport.SetBluetoothDialer(m.Dial)
	a.discovery.SetExtraSource(func() []discovery.ExtraPeer {
		peers := m.Peers()
		out := make([]discovery.ExtraPeer, 0, len(peers))
		for _, p := range peers {
			out = append(out, discovery.ExtraPeer{DeviceID: p.DeviceID, Name: p.Name, Address: p.Address})
		}
		return out
	})
	m.OnChange(func() {
		wailsRuntime.EventsEmit(a.ctx, "bluetooth-updated", m.Status())
		wailsRuntime.EventsEmit(a.ctx, "peers-updated", a.GetPeers())
	})
	SafeGoRestart(a.ctx, "bluetooth", func(ctx context.Context) {
		m.Run(ctx, func(l net.Listener) {
			if err := a.transport.Serve(l); err != nil {
				log.Printf("bluetooth: serve: %v", err)
				l.Close()
			}
		})
	})
}

// GetBluetoothStatus reports the state of the native Bluetooth link and the
// paired PCs found running MultiSnek.
func (a *App) GetBluetoothStatus() bluetooth.Status {
	if a.bluetooth == nil {
		return bluetooth.Status{Devices: []bluetooth.Peer{}, Error: bluetooth.ErrUnavailable.Error()}
	}
	return a.bluetooth.Status()
}

// SetBluetoothEnabled turns the native Bluetooth link on or off.
func (a *App) SetBluetoothEnabled(enabled bool) {
	a.settings.Update(func(s *settings.Settings) { s.BluetoothEnabled = &enabled })
	if a.bluetooth != nil {
		a.bluetooth.SetEnabled(enabled)
	}
}

// RefreshBluetooth rescans paired devices now.
func (a *App) RefreshBluetooth() {
	if a.bluetooth != nil {
		a.bluetooth.Refresh()
	}
}
