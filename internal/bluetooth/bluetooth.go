// Package bluetooth carries MultiSnek's normal TLS session over a direct
// Classic Bluetooth (RFCOMM) link between two paired Windows PCs, with no
// network setup. Each PC registers an SDP service whose name carries its
// device ID, so paired PCs running MultiSnek can be found without a network.
//
// The platform stack is behind Backend; Manager (this file) is portable and
// unit-tested with a fake backend.
package bluetooth

import (
	"errors"
	"fmt"
	"strings"
)

// Scheme prefixes Bluetooth peer addresses ("bt://AA:BB:CC:DD:EE:FF") so they
// can live next to host:port addresses everywhere an address string is used.
const Scheme = "bt://"

// ErrUnavailable means this PC has no usable Bluetooth stack or radio.
var ErrUnavailable = errors.New("bluetooth is not available on this PC")

// ServiceClassID identifies the MultiSnek RFCOMM service in SDP.
const ServiceClassID = "6d756c74-6973-6e65-6b4b-564d00000001"

const serviceNamePrefix = "MultiSnek|"

// IsAddress reports whether s is a Bluetooth peer address.
func IsAddress(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), Scheme) }

// ParseAddress accepts "bt://AA:BB:CC:DD:EE:FF" or the bare form and returns
// the 48-bit device address.
func ParseAddress(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), Scheme)
	parts := strings.Split(strings.Trim(s, "()"), ":")
	if len(parts) != 6 {
		return 0, fmt.Errorf("invalid Bluetooth address %q", s)
	}
	var addr uint64
	for _, p := range parts {
		if len(p) != 2 {
			return 0, fmt.Errorf("invalid Bluetooth address %q", s)
		}
		var b byte
		if _, err := fmt.Sscanf(p, "%02x", &b); err != nil {
			return 0, fmt.Errorf("invalid Bluetooth address %q", s)
		}
		addr = addr<<8 | uint64(b)
	}
	return addr, nil
}

// FormatAddress renders a device address as "AA:BB:CC:DD:EE:FF".
func FormatAddress(addr uint64) string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		byte(addr>>40), byte(addr>>32), byte(addr>>24), byte(addr>>16), byte(addr>>8), byte(addr))
}

// NormalizeAddress returns the canonical "bt://AA:BB:CC:DD:EE:FF" form.
func NormalizeAddress(s string) (string, error) {
	a, err := ParseAddress(s)
	if err != nil {
		return "", err
	}
	return Scheme + FormatAddress(a), nil
}

// EncodeServiceName builds the SDP service name advertised by this PC.
func EncodeServiceName(deviceID, deviceName string) string {
	clean := func(s string) string {
		s = strings.Map(func(r rune) rune {
			if r == '|' || r < 0x20 {
				return -1
			}
			return r
		}, s)
		if r := []rune(s); len(r) > 60 {
			s = string(r[:60])
		}
		return s
	}
	return serviceNamePrefix + clean(deviceID) + "|" + clean(deviceName)
}

// DecodeServiceName extracts the device ID and name from an SDP service name.
func DecodeServiceName(s string) (deviceID, deviceName string, ok bool) {
	rest, found := strings.CutPrefix(s, serviceNamePrefix)
	if !found {
		return "", "", false
	}
	id, name, found := strings.Cut(rest, "|")
	if !found || id == "" {
		return "", "", false
	}
	return id, name, true
}
