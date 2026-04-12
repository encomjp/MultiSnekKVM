// testpeer — standalone CLI tool that speaks the MultiSnekKVM protocol.
// Usage:
//
//	go run ./tools/testpeer -listen :24832                # listen mode
//	go run ./tools/testpeer -connect 192.168.0.68:24831   # connect to a peer
//	go run ./tools/testpeer -connect :24831               # connect to localhost
//
// It generates its own ephemeral identity (cert + device ID) in a temp dir,
// performs the full TLS 1.3 + hello handshake, and logs every step so you
// can diagnose trust / fingerprint / protocol issues.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"
)

// ─── protocol (mirrors main app) ─────────────────────────────────────────────

const (
	MsgHello     byte = 0x01
	MsgHeartbeat byte = 0x03
	MsgPing      byte = 0x04
	MsgPong      byte = 0x05
)

type Frame struct {
	Type    byte
	Payload []byte
}

func writeFrame(w io.Writer, f Frame) error {
	header := make([]byte, 5)
	header[0] = f.Type
	binary.BigEndian.PutUint32(header[1:], uint32(len(f.Payload)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(f.Payload) > 0 {
		_, err := w.Write(f.Payload)
		return err
	}
	return nil
}

func readFrame(r io.Reader) (Frame, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return Frame{}, err
	}
	length := binary.BigEndian.Uint32(header[1:])
	if length > 1<<20 {
		return Frame{}, fmt.Errorf("frame too large: %d", length)
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
	}
	return Frame{Type: header[0], Payload: payload}, nil
}

type HelloMsg struct {
	DeviceID    string
	Name        string
	Fingerprint string
}

func (m HelloMsg) Encode() []byte {
	return []byte(m.DeviceID + "\n" + m.Name + "\n" + m.Fingerprint)
}

func decodeHello(b []byte) (HelloMsg, error) {
	parts := strings.SplitN(string(b), "\n", 3)
	if len(parts) != 3 {
		return HelloMsg{}, fmt.Errorf("hello payload malformed: got %d parts", len(parts))
	}
	return HelloMsg{DeviceID: parts[0], Name: parts[1], Fingerprint: parts[2]}, nil
}

// ─── identity ─────────────────────────────────────────────────────────────────

type testIdentity struct {
	DeviceID    string
	Name        string
	Fingerprint string
	TLSCert     tls.Certificate
}

func generateIdentity(name string) (*testIdentity, error) {
	// random device ID
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	deviceID := hex.EncodeToString(buf)

	// ECDSA P-256 key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   name,
			Organization: []string{"MultiSnekKVM"},
			SerialNumber: deviceID,
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	cert, _ := x509.ParseCertificate(certDER)
	fp := sha256.Sum256(cert.Raw)

	return &testIdentity{
		DeviceID:    deviceID,
		Name:        name,
		Fingerprint: hex.EncodeToString(fp[:]),
		TLSCert:     tlsCert,
	}, nil
}

// ─── handshake ────────────────────────────────────────────────────────────────

func doHandshake(conn *tls.Conn, id *testIdentity) (*HelloMsg, string, error) {
	log.Println("  [1] TLS handshake...")
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := conn.Handshake(); err != nil {
		return nil, "", fmt.Errorf("TLS handshake failed: %w", err)
	}
	state := conn.ConnectionState()
	log.Printf("  [2] TLS version: 0x%04x, cipher: 0x%04x", state.Version, state.CipherSuite)

	if len(state.PeerCertificates) == 0 {
		return nil, "", fmt.Errorf("peer presented no certificate")
	}
	peerCert := state.PeerCertificates[0]
	peerFP := sha256.Sum256(peerCert.Raw)
	peerFingerprint := hex.EncodeToString(peerFP[:])
	log.Printf("  [3] Peer cert: CN=%q Org=%v Serial=%q", peerCert.Subject.CommonName, peerCert.Subject.Organization, peerCert.Subject.SerialNumber)
	log.Printf("  [4] Peer cert fingerprint: %s", peerFingerprint)

	// Send our hello
	hello := HelloMsg{
		DeviceID:    id.DeviceID,
		Name:        id.Name,
		Fingerprint: id.Fingerprint,
	}
	log.Printf("  [5] Sending hello: device=%s name=%s fp=%s", hello.DeviceID, hello.Name, hello.Fingerprint[:16]+"...")
	if err := writeFrame(conn, Frame{Type: MsgHello, Payload: hello.Encode()}); err != nil {
		return nil, "", fmt.Errorf("send hello: %w", err)
	}

	// Read peer hello
	log.Println("  [6] Waiting for peer hello...")
	frame, err := readFrame(conn)
	if err != nil {
		return nil, "", fmt.Errorf("read peer hello: %w", err)
	}
	if frame.Type != MsgHello {
		return nil, "", fmt.Errorf("expected MsgHello (0x01), got 0x%02x", frame.Type)
	}
	peerHello, err := decodeHello(frame.Payload)
	if err != nil {
		return nil, "", fmt.Errorf("decode peer hello: %w", err)
	}
	log.Printf("  [7] Peer hello: device=%s name=%s fp=%s", peerHello.DeviceID, peerHello.Name, peerHello.Fingerprint[:16]+"...")

	// Cross-validate hello vs cert
	log.Println("  [8] Validating peer hello against certificate...")
	if !containsStringFold(peerCert.Subject.Organization, "MultiSnekKVM") {
		return nil, "", fmt.Errorf("FAIL: peer cert org %v does not contain MultiSnekKVM", peerCert.Subject.Organization)
	}
	if peerHello.DeviceID != strings.TrimSpace(peerCert.Subject.SerialNumber) {
		return nil, "", fmt.Errorf("FAIL: hello device_id %q != cert serial %q", peerHello.DeviceID, peerCert.Subject.SerialNumber)
	}
	if !strings.EqualFold(peerHello.Name, strings.TrimSpace(peerCert.Subject.CommonName)) {
		return nil, "", fmt.Errorf("FAIL: hello name %q != cert CN %q", peerHello.Name, peerCert.Subject.CommonName)
	}
	if !strings.EqualFold(peerHello.Fingerprint, peerFingerprint) {
		return nil, "", fmt.Errorf("FAIL: hello fingerprint != cert fingerprint")
	}
	log.Println("  [9] Validation PASSED")

	conn.SetDeadline(time.Time{})
	return &peerHello, peerFingerprint, nil
}

// ─── session loop ─────────────────────────────────────────────────────────────

func sessionLoop(conn *tls.Conn, peerName string) {
	// heartbeat sender
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := writeFrame(conn, Frame{Type: MsgHeartbeat}); err != nil {
				log.Printf("  heartbeat send error: %v", err)
				return
			}
		}
	}()

	// read loop
	for {
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		frame, err := readFrame(conn)
		if err != nil {
			if err == io.EOF {
				log.Printf("  peer %s disconnected (EOF)", peerName)
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("  peer %s heartbeat timeout", peerName)
			} else {
				log.Printf("  read error from %s: %v", peerName, err)
			}
			return
		}

		typeName := fmt.Sprintf("0x%02x", frame.Type)
		switch frame.Type {
		case MsgHeartbeat:
			typeName = "heartbeat"
		case MsgPing:
			typeName = "ping"
			// respond with pong
			writeFrame(conn, Frame{Type: MsgPong, Payload: frame.Payload})
		case MsgPong:
			typeName = "pong"
		}
		log.Printf("  ← %s from %s (%d bytes payload)", typeName, peerName, len(frame.Payload))
	}
}

// ─── modes ────────────────────────────────────────────────────────────────────

func runListen(addr string, id *testIdentity) {
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{id.TLSCert},
		ClientAuth:   tls.RequireAnyClientCert,
	}
	listener, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	log.Printf("LISTEN on %s", listener.Addr())
	log.Printf("  device=%s name=%s fp=%s", id.DeviceID, id.Name, id.Fingerprint[:16]+"...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		tlsConn := conn.(*tls.Conn)
		log.Printf("INBOUND connection from %s", conn.RemoteAddr())
		go func() {
			defer tlsConn.Close()
			peerHello, _, err := doHandshake(tlsConn, id)
			if err != nil {
				log.Printf("  HANDSHAKE FAILED: %v", err)
				return
			}
			log.Printf("  SESSION ESTABLISHED with %s (%s) — we are 'controlled'", peerHello.Name, peerHello.DeviceID[:12])
			sessionLoop(tlsConn, peerHello.Name)
			log.Printf("  SESSION ENDED with %s", peerHello.Name)
		}()
	}
}

func runConnect(addr string, id *testIdentity) {
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{id.TLSCert},
		InsecureSkipVerify: true, // TOFU — validation via hello cross-check
	}
	log.Printf("CONNECT to %s", addr)
	log.Printf("  device=%s name=%s fp=%s", id.DeviceID, id.Name, id.Fingerprint[:16]+"...")

	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		log.Fatalf("connect %s: %v", addr, err)
	}
	defer conn.Close()

	peerHello, _, err := doHandshake(conn, id)
	if err != nil {
		log.Fatalf("  HANDSHAKE FAILED: %v", err)
	}
	log.Printf("  SESSION ESTABLISHED with %s (%s) — we are 'controller'", peerHello.Name, peerHello.DeviceID[:12])
	sessionLoop(conn, peerHello.Name)
	log.Printf("  SESSION ENDED with %s", peerHello.Name)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func containsStringFold(values []string, candidate string) bool {
	for _, v := range values {
		if strings.EqualFold(v, candidate) {
			return true
		}
	}
	return false
}

// ─── main ─────────────────────────────────────────────────────────────────────

func main() {
	listenAddr := flag.String("listen", "", "listen address (e.g. :24832)")
	connectAddr := flag.String("connect", "", "connect to address (e.g. 192.168.0.68:24831)")
	name := flag.String("name", "", "device name (default: hostname)")
	flag.Parse()

	if *listenAddr == "" && *connectAddr == "" {
		fmt.Fprintln(os.Stderr, "usage: testpeer -listen :24832  OR  testpeer -connect host:port")
		flag.PrintDefaults()
		os.Exit(1)
	}

	peerName := *name
	if peerName == "" {
		peerName, _ = os.Hostname()
		if peerName == "" {
			peerName = "testpeer"
		}
	}

	id, err := generateIdentity(peerName)
	if err != nil {
		log.Fatalf("generate identity: %v", err)
	}
	log.Printf("Generated ephemeral identity: device=%s name=%s", id.DeviceID, peerName)
	log.Printf("  fingerprint=%s", id.Fingerprint)

	// Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		log.Println("interrupted, exiting")
		os.Exit(0)
	}()

	if *listenAddr != "" {
		runListen(*listenAddr, id)
	} else {
		runConnect(*connectAddr, id)
	}
}
