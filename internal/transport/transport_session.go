package transport

import (
	"io"
	"log"
	"net"
	"time"

	"multisnekkvm/internal/protocol"
)

func (t *Transport) readLoop(s *Session) {
	defer func() {
		t.mu.Lock()
		wasActive := t.session == s
		if wasActive {
			t.session = nil
		}
		t.mu.Unlock()
		s.Close()
		log.Printf("connection to %s closed", s.PeerName)
		if wasActive && t.OnDisconnect != nil {
			safeCall("OnDisconnect/readLoop", t.OnDisconnect)
		}
	}()

	for {
		select {
		case <-s.closeCh:
			return
		default:
		}

		_ = s.conn.SetReadDeadline(time.Now().Add(heartbeatTimeout))
		frame, err := protocol.ReadFrame(s.conn)
		if err != nil {
			if err != io.EOF {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Printf("heartbeat timeout from %s", s.PeerName)
				} else {
					log.Printf("read error from %s: %v", s.PeerName, err)
				}
			}
			return
		}
		_ = s.conn.SetReadDeadline(time.Time{})

		if frame.Type == protocol.MsgHeartbeat {
			continue
		}

		if t.OnFrame != nil {
			safeCall("OnFrame", func() { t.OnFrame(frame) })
		}
	}
}
