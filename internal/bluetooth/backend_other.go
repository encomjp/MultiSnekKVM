//go:build !windows

package bluetooth

import (
	"context"
	"net"
	"time"
)

// NewBackend returns a backend that reports Bluetooth as unavailable;
// native Bluetooth transport is implemented for Windows only.
func NewBackend() Backend { return unsupported{} }

type unsupported struct{}

func (unsupported) Available() error                    { return ErrUnavailable }
func (unsupported) Listen(string) (net.Listener, error) { return nil, ErrUnavailable }
func (unsupported) Dial(string, time.Duration) (net.Conn, error) {
	return nil, ErrUnavailable
}
func (unsupported) Scan(context.Context) ([]Peer, error) { return nil, ErrUnavailable }
