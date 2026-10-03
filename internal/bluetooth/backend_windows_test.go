//go:build windows

package bluetooth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// socketPair returns two connected btConns. They run over a loopback TCP
// socket rather than Bluetooth: btConn only uses recv/send/closesocket, which
// behave the same on any Winsock stream socket, so this exercises the real
// deadline, close and blocking behaviour without a radio.
func socketPair(t *testing.T) (a, b *btConn) {
	t.Helper()
	if err := initWinsock(); err != nil {
		t.Skipf("winsock unavailable: %v", err)
	}
	ln, err := windows.Socket(windows.AF_INET, windows.SOCK_STREAM, windows.IPPROTO_TCP)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.Closesocket(ln)
	if err := windows.Bind(ln, &windows.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Fatal(err)
	}
	if err := windows.Listen(ln, 1); err != nil {
		t.Fatal(err)
	}
	sa, err := windows.Getsockname(ln)
	if err != nil {
		t.Fatal(err)
	}
	port := sa.(*windows.SockaddrInet4).Port

	cl, err := windows.Socket(windows.AF_INET, windows.SOCK_STREAM, windows.IPPROTO_TCP)
	if err != nil {
		t.Fatal(err)
	}
	connected := make(chan error, 1)
	go func() {
		connected <- windows.Connect(cl, &windows.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}, Port: port})
	}()
	r, _, e := pAccept.Call(uintptr(ln), 0, 0)
	if windows.Handle(r) == windows.InvalidHandle {
		t.Fatalf("accept: %v", e)
	}
	if err := <-connected; err != nil {
		t.Fatal(err)
	}
	a = newConn(windows.Handle(r), "bt://local", "bt://00:00:00:00:00:0B")
	b = newConn(cl, "bt://local", "bt://00:00:00:00:00:0A")
	t.Cleanup(func() { a.Close(); b.Close() })
	return a, b
}

func TestConnReadWrite(t *testing.T) {
	a, b := socketPair(t)
	if _, err := a.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	n, err := b.Read(buf)
	if err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("read %q err=%v", buf[:n], err)
	}
	if a.RemoteAddr().String() != "bt://00:00:00:00:00:0B" || a.RemoteAddr().Network() != "bluetooth" {
		t.Fatalf("remote addr %v", a.RemoteAddr())
	}
}

func TestConnLargeWriteWithConcurrentReader(t *testing.T) {
	a, b := socketPair(t)
	const total = 4 << 20
	go func() {
		chunk := make([]byte, 64<<10)
		for sent := 0; sent < total; sent += len(chunk) {
			if _, err := a.Write(chunk); err != nil {
				return
			}
		}
		a.Close()
	}()
	got := 0
	buf := make([]byte, 32<<10)
	for {
		n, err := b.Read(buf)
		got += n
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("read error after %d bytes: %v", got, err)
			}
			break
		}
	}
	if got != total {
		t.Fatalf("received %d of %d bytes", got, total)
	}
}

func TestConnReadDeadlineTimesOutAndConnectionSurvives(t *testing.T) {
	a, b := socketPair(t)
	b.SetReadDeadline(time.Now().Add(120 * time.Millisecond))
	start := time.Now()
	_, err := b.Read(make([]byte, 4))
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if el := time.Since(start); el < 80*time.Millisecond || el > 2*time.Second {
		t.Fatalf("timeout after %v", el)
	}

	// An already-expired deadline fails immediately; clearing it restores use.
	b.SetReadDeadline(time.Now().Add(-time.Second))
	if _, err := b.Read(make([]byte, 4)); !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("expired deadline: %v", err)
	}
	b.SetReadDeadline(time.Time{})
	a.Write([]byte("ok"))
	buf := make([]byte, 4)
	if n, err := b.Read(buf); err != nil || string(buf[:n]) != "ok" {
		t.Fatalf("after timeout: %q %v", buf[:n], err)
	}
}

func TestConnCloseUnblocksPendingRead(t *testing.T) {
	_, b := socketPair(t)
	res := make(chan error, 1)
	go func() { _, err := b.Read(make([]byte, 4)); res <- err }()
	time.Sleep(100 * time.Millisecond)
	b.Close()
	select {
	case err := <-res:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("expected net.ErrClosed, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not unblock a pending Read")
	}
	if _, err := b.Write([]byte("x")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after close: %v", err)
	}
}

func TestConnPeerCloseIsEOF(t *testing.T) {
	a, b := socketPair(t)
	a.Close()
	if _, err := b.Read(make([]byte, 4)); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

// The real transport runs TLS 1.3 with mutual certificates over this conn.
func TestTLSOverConn(t *testing.T) {
	a, b := socketPair(t)
	cert := selfSigned(t)
	srv := tls.Server(a, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAnyClientCert})
	cli := tls.Client(b, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true})

	done := make(chan error, 1)
	go func() {
		if err := srv.Handshake(); err != nil {
			done <- err
			return
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(srv, buf); err != nil {
			done <- err
			return
		}
		_, err := srv.Write([]byte("pong:" + string(buf)))
		done <- err
	}()
	cli.SetDeadline(time.Now().Add(5 * time.Second))
	if err := cli.Handshake(); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if _, err := cli.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(cli, reply); err != nil || string(reply) != "pong:hello" {
		t.Fatalf("reply %q err=%v", reply, err)
	}
	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "multisnek-test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestSockaddrBthLayoutMatchesWin32(t *testing.T) {
	if got := unsafe.Sizeof(sockaddrBth{}); got != sockaddrBthSize {
		t.Fatalf("sockaddrBth is %d bytes, SOCKADDR_BTH is %d", got, sockaddrBthSize)
	}
	var sa sockaddrBth
	sa.BtAddr = [8]byte{0x13, 0x71, 0xDA, 0x7D, 0x1A, 0x00, 0, 0}
	if FormatAddress(sa.addr()) != "00:1A:7D:DA:71:13" {
		t.Fatalf("address decode: %s", FormatAddress(sa.addr()))
	}
}

func TestBackendAvailableDoesNotPanic(t *testing.T) {
	// Result depends on the machine's radio; it must simply return.
	err := NewBackend().Available()
	if err != nil && !errors.Is(err, ErrUnavailable) {
		t.Logf("unavailable: %v", err)
	}
}
