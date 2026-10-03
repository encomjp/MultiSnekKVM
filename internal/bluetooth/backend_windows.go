//go:build windows

package bluetooth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewBackend returns the Windows Winsock (AF_BTH / RFCOMM) backend.
func NewBackend() Backend { return &winBackend{checked: map[uint64]checkedDevice{}} }

const (
	btPortAny       = 0xFFFFFFFF // BT_PORT_ANY
	rnrRegister     = 0          // RNRSERVICE_REGISTER
	rnrDelete       = 2          // RNRSERVICE_DELETE
	soSndTimeo      = 0x1005     // SO_SNDTIMEO
	wsaNoMore       = 10110      // WSA_E_NO_MORE / WSAENOMORE
	wsaNotFound     = 10108      // WSASERVICE_NOT_FOUND
	wsaFault        = 10014      // WSAEFAULT: buffer too small
	wsaNoData       = 11004
	sockaddrBthSize = 30
	negativeTTL     = 10 * time.Minute
	scanWorkers     = 4
)

var (
	ws2            = syscall.NewLazyDLL("ws2_32.dll")
	pAccept        = ws2.NewProc("accept")
	pRecv          = ws2.NewProc("recv")
	pSend          = ws2.NewProc("send")
	pSetsockopt    = ws2.NewProc("setsockopt")
	pGetsockname   = ws2.NewProc("getsockname")
	pWSASetService = ws2.NewProc("WSASetService")

	serviceGUID = windows.GUID{Data1: 0x6d756c74, Data2: 0x6973, Data3: 0x6e65, Data4: [8]byte{0x6b, 0x4b, 0x56, 0x4d, 0x00, 0x00, 0x00, 0x01}}

	wsaOnce sync.Once
	wsaErr  error
)

// sockaddrBth mirrors the packed Win32 SOCKADDR_BTH (30 bytes).
type sockaddrBth struct {
	Family         [2]byte
	BtAddr         [8]byte
	ServiceClassID [16]byte
	Port           [4]byte
}

func (s *sockaddrBth) addr() uint64 {
	var a uint64
	for i := 7; i >= 0; i-- {
		a = a<<8 | uint64(s.BtAddr[i])
	}
	return a
}

func initWinsock() error {
	wsaOnce.Do(func() {
		var data windows.WSAData
		wsaErr = windows.WSAStartup(0x0202, &data)
	})
	return wsaErr
}

type winBackend struct {
	mu      sync.Mutex
	checked map[uint64]checkedDevice
}

type checkedDevice struct {
	at   time.Time
	peer *Peer // nil: reachable-or-not, but no MultiSnek service
}

func (b *winBackend) Available() error {
	if err := initWinsock(); err != nil {
		return fmt.Errorf("%w: winsock: %v", ErrUnavailable, err)
	}
	fd, err := windows.Socket(windows.AF_BTH, windows.SOCK_STREAM, windows.BTHPROTO_RFCOMM)
	if err != nil {
		return ErrUnavailable
	}
	windows.Closesocket(fd)
	return nil
}

// --- connections ---------------------------------------------------------

type btAddrString string

func (a btAddrString) Network() string { return "bluetooth" }
func (a btAddrString) String() string  { return string(a) }

type timeoutError struct{}

func (timeoutError) Error() string   { return "bluetooth i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type btConn struct {
	fd            windows.Handle
	local, remote btAddrString
	closed        atomic.Bool
	rdl, wdl      atomic.Int64 // deadline, UnixNano; 0 = none
	rdApplied     atomic.Int64 // last SO_RCVTIMEO applied (ms), -1 = unset
	wrApplied     atomic.Int64
}

func newConn(fd windows.Handle, local, remote string) *btConn {
	c := &btConn{fd: fd, local: btAddrString(local), remote: btAddrString(remote)}
	c.rdApplied.Store(-1)
	c.wrApplied.Store(-1)
	return c
}

func (c *btConn) timeoutFor(deadline *atomic.Int64) (ms int64, expired bool) {
	d := deadline.Load()
	if d == 0 {
		return 0, false
	}
	left := time.Until(time.Unix(0, d))
	if left <= 0 {
		return 0, true
	}
	ms = left.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	return ms, false
}

func (c *btConn) applyTimeout(opt int32, ms int64, applied *atomic.Int64) {
	if applied.Load() == ms {
		return
	}
	v := uint32(ms)
	pSetsockopt.Call(uintptr(c.fd), uintptr(windows.SOL_SOCKET), uintptr(opt), uintptr(unsafe.Pointer(&v)), 4)
	applied.Store(ms)
}

func (c *btConn) wrapErr(op string, err error) error {
	if c.closed.Load() {
		return net.ErrClosed
	}
	if errors.Is(err, windows.WSAETIMEDOUT) {
		return timeoutError{}
	}
	return &net.OpError{Op: op, Net: "bluetooth", Addr: c.remote, Err: err}
}

func (c *btConn) Read(p []byte) (int, error) {
	if c.closed.Load() {
		return 0, net.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	ms, expired := c.timeoutFor(&c.rdl)
	if expired {
		return 0, timeoutError{}
	}
	c.applyTimeout(windows.SO_RCVTIMEO, ms, &c.rdApplied)
	r, _, e := pRecv.Call(uintptr(c.fd), uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)), 0)
	n := int32(r)
	if n == -1 {
		return 0, c.wrapErr("read", e)
	}
	if n == 0 {
		if c.closed.Load() {
			return 0, net.ErrClosed
		}
		return 0, io.EOF
	}
	return int(n), nil
}

func (c *btConn) Write(p []byte) (int, error) {
	written := 0
	for written < len(p) {
		if c.closed.Load() {
			return written, net.ErrClosed
		}
		ms, expired := c.timeoutFor(&c.wdl)
		if expired {
			return written, timeoutError{}
		}
		c.applyTimeout(soSndTimeo, ms, &c.wrApplied)
		chunk := p[written:]
		r, _, e := pSend.Call(uintptr(c.fd), uintptr(unsafe.Pointer(&chunk[0])), uintptr(len(chunk)), 0)
		n := int32(r)
		if n == -1 {
			return written, c.wrapErr("write", e)
		}
		written += int(n)
	}
	return written, nil
}

func (c *btConn) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	// closesocket from another thread unblocks a pending recv/send.
	return windows.Closesocket(c.fd)
}

func (c *btConn) LocalAddr() net.Addr  { return c.local }
func (c *btConn) RemoteAddr() net.Addr { return c.remote }

// Deadlines apply to the next Read/Write; they do not interrupt one that is
// already blocked (the session closes the connection to do that).
func (c *btConn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	return c.SetWriteDeadline(t)
}

func storeDeadline(a *atomic.Int64, t time.Time) {
	if t.IsZero() {
		a.Store(0)
		return
	}
	a.Store(t.UnixNano())
}

func (c *btConn) SetReadDeadline(t time.Time) error  { storeDeadline(&c.rdl, t); return nil }
func (c *btConn) SetWriteDeadline(t time.Time) error { storeDeadline(&c.wdl, t); return nil }

// --- dial ----------------------------------------------------------------

func (b *winBackend) Dial(address string, timeout time.Duration) (net.Conn, error) {
	if err := initWinsock(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	addr, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}
	fd, err := windows.Socket(windows.AF_BTH, windows.SOCK_STREAM, windows.BTHPROTO_RFCOMM)
	if err != nil {
		return nil, ErrUnavailable
	}
	remote := Scheme + FormatAddress(addr)
	conn := newConn(fd, "bt://local", remote)

	// Port 0 plus the service class ID makes Windows look the RFCOMM channel
	// up through SDP on the remote device.
	sa := &windows.SockaddrBth{BtAddr: addr, ServiceClassId: serviceGUID, Port: 0}
	done := make(chan error, 1)
	go func() { done <- windows.Connect(fd, sa) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("bluetooth connect %s: %w", remote, err)
		}
		return conn, nil
	case <-timer.C:
		conn.Close() // aborts the pending connect
		<-done
		return nil, fmt.Errorf("bluetooth connect %s: timed out after %v", remote, timeout)
	}
}

// --- listen --------------------------------------------------------------

type btListener struct {
	fd      windows.Handle
	closed  atomic.Bool
	qs      windows.WSAQUERYSET // kept alive until unregistered
	csa     windows.CSAddrInfo
	local   sockaddrBth
	name    *uint16
	class   windows.GUID
	regOnce sync.Once
}

func (b *winBackend) Listen(serviceName string) (net.Listener, error) {
	if err := initWinsock(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	fd, err := windows.Socket(windows.AF_BTH, windows.SOCK_STREAM, windows.BTHPROTO_RFCOMM)
	if err != nil {
		return nil, ErrUnavailable
	}
	sa := &windows.SockaddrBth{Port: btPortAny}
	if err := windows.Bind(fd, sa); err != nil {
		windows.Closesocket(fd)
		return nil, fmt.Errorf("bluetooth bind: %w", err)
	}
	l := &btListener{fd: fd, class: serviceGUID}
	size := int32(sockaddrBthSize)
	if r, _, e := pGetsockname.Call(uintptr(fd), uintptr(unsafe.Pointer(&l.local)), uintptr(unsafe.Pointer(&size))); int32(r) != 0 {
		windows.Closesocket(fd)
		return nil, fmt.Errorf("bluetooth getsockname: %w", e)
	}
	if err := windows.Listen(fd, 2); err != nil {
		windows.Closesocket(fd)
		return nil, fmt.Errorf("bluetooth listen: %w", err)
	}

	// Advertise the service (class ID, RFCOMM channel and a name carrying
	// our device ID) in SDP so paired PCs can find and connect to it.
	l.name = windows.StringToUTF16Ptr(serviceName)
	l.csa = windows.CSAddrInfo{
		LocalAddr:  windows.SocketAddress{Sockaddr: (*syscall.RawSockaddrAny)(unsafe.Pointer(&l.local)), SockaddrLength: sockaddrBthSize},
		SocketType: windows.SOCK_STREAM,
		Protocol:   windows.BTHPROTO_RFCOMM,
	}
	l.qs = windows.WSAQUERYSET{
		Size:                uint32(unsafe.Sizeof(windows.WSAQUERYSET{})),
		ServiceInstanceName: l.name,
		ServiceClassId:      &l.class,
		NameSpace:           windows.NS_BTH,
		NumberOfCsAddrs:     1,
		SaBuffer:            &l.csa,
	}
	if r, _, e := pWSASetService.Call(uintptr(unsafe.Pointer(&l.qs)), rnrRegister, 0); int32(r) != 0 {
		windows.Closesocket(fd)
		return nil, fmt.Errorf("bluetooth register service: %w", e)
	}
	return l, nil
}

func (l *btListener) Accept() (net.Conn, error) {
	var raw sockaddrBth
	size := int32(sockaddrBthSize)
	r, _, e := pAccept.Call(uintptr(l.fd), uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&size)))
	if r == ^uintptr(0) || windows.Handle(r) == windows.InvalidHandle {
		if l.closed.Load() {
			return nil, net.ErrClosed
		}
		return nil, &net.OpError{Op: "accept", Net: "bluetooth", Err: e}
	}
	remote := Scheme + FormatAddress(raw.addr())
	return newConn(windows.Handle(r), "bt://local", remote), nil
}

func (l *btListener) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}
	l.regOnce.Do(func() {
		pWSASetService.Call(uintptr(unsafe.Pointer(&l.qs)), rnrDelete, 0)
	})
	return windows.Closesocket(l.fd)
}

func (l *btListener) Addr() net.Addr { return btAddrString("bt://local") }

// --- scan ----------------------------------------------------------------

type device struct {
	addr uint64
	name string
}

// lookupBuf is 8-byte aligned storage for WSAQUERYSET results.
func lookupBuf(n int) ([]uint64, unsafe.Pointer, int32) {
	words := (n + 7) / 8
	b := make([]uint64, words)
	return b, unsafe.Pointer(&b[0]), int32(words * 8)
}

func queryNext(h windows.Handle) (*windows.WSAQUERYSET, []uint64, error) {
	size := 4096
	for attempt := 0; attempt < 4; attempt++ {
		buf, p, n := lookupBuf(size)
		qs := (*windows.WSAQUERYSET)(p)
		err := windows.WSALookupServiceNext(h, 0, &n, qs)
		if err == nil {
			return qs, buf, nil
		}
		if errors.Is(err, syscall.Errno(wsaFault)) && int(n) > size {
			size = int(n)
			continue
		}
		return nil, nil, err
	}
	return nil, nil, errors.New("lookup buffer kept growing")
}

func isNoMore(err error) bool {
	return errors.Is(err, syscall.Errno(wsaNoMore)) || errors.Is(err, syscall.Errno(wsaNotFound)) || errors.Is(err, syscall.Errno(wsaNoData))
}

// remembered lists known (paired or previously seen) devices without
// triggering a slow radio inquiry.
func (b *winBackend) remembered() ([]device, error) {
	var qs windows.WSAQUERYSET
	qs.Size = uint32(unsafe.Sizeof(qs))
	qs.NameSpace = windows.NS_BTH
	var h windows.Handle
	flags := uint32(windows.LUP_CONTAINERS | windows.LUP_RETURN_NAME | windows.LUP_RETURN_ADDR)
	if err := windows.WSALookupServiceBegin(&qs, flags, &h); err != nil {
		if isNoMore(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list Bluetooth devices: %w", err)
	}
	defer windows.WSALookupServiceEnd(h)

	var devs []device
	for {
		res, buf, err := queryNext(h)
		if err != nil {
			if isNoMore(err) {
				return devs, nil
			}
			return devs, fmt.Errorf("list Bluetooth devices: %w", err)
		}
		if res.SaBuffer != nil && res.SaBuffer.RemoteAddr.Sockaddr != nil {
			sa := (*sockaddrBth)(unsafe.Pointer(res.SaBuffer.RemoteAddr.Sockaddr))
			d := device{addr: sa.addr()}
			if res.ServiceInstanceName != nil {
				d.name = windows.UTF16PtrToString(res.ServiceInstanceName)
			}
			devs = append(devs, d)
		}
		_ = buf // keep the result buffer alive until parsed
	}
}

// queryService asks a device's SDP server whether it hosts MultiSnek and
// returns the advertised service name.
func (b *winBackend) queryService(d device) (*Peer, error) {
	ctx := windows.StringToUTF16Ptr("(" + FormatAddress(d.addr) + ")")
	class := serviceGUID
	qs := windows.WSAQUERYSET{
		Size:           uint32(unsafe.Sizeof(windows.WSAQUERYSET{})),
		ServiceClassId: &class,
		NameSpace:      windows.NS_BTH,
		Context:        ctx,
	}
	var h windows.Handle
	if err := windows.WSALookupServiceBegin(&qs, windows.LUP_RETURN_ALL, &h); err != nil {
		if isNoMore(err) {
			return nil, nil
		}
		return nil, err
	}
	defer windows.WSALookupServiceEnd(h)
	res, buf, err := queryNext(h)
	if err != nil {
		if isNoMore(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = buf }()

	address := Scheme + FormatAddress(d.addr)
	peer := &Peer{Address: address, DeviceID: "bt:" + FormatAddress(d.addr), Name: d.name}
	if res.ServiceInstanceName != nil {
		if id, name, ok := DecodeServiceName(windows.UTF16PtrToString(res.ServiceInstanceName)); ok {
			peer.DeviceID = id
			if name != "" {
				peer.Name = name
			}
		}
	}
	if peer.Name == "" {
		peer.Name = FormatAddress(d.addr)
	}
	return peer, nil
}

func (b *winBackend) Scan(ctx context.Context) ([]Peer, error) {
	if err := initWinsock(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	devs, err := b.remembered()
	if err != nil && len(devs) == 0 {
		return nil, err
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		found []Peer
		sem   = make(chan struct{}, scanWorkers)
		now   = time.Now()
	)
	for _, d := range devs {
		b.mu.Lock()
		c, seen := b.checked[d.addr]
		b.mu.Unlock()
		if seen && c.peer == nil && now.Sub(c.at) < negativeTTL {
			continue // recently checked: no MultiSnek there
		}
		wg.Add(1)
		go func(d device) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if ctx.Err() != nil {
				return
			}
			peer, qerr := b.queryService(d)
			if qerr != nil {
				// Unreachable now (out of range, off): not cached, retried next scan.
				return
			}
			b.mu.Lock()
			b.checked[d.addr] = checkedDevice{at: time.Now(), peer: peer}
			b.mu.Unlock()
			if peer != nil {
				mu.Lock()
				found = append(found, *peer)
				mu.Unlock()
			}
		}(d)
	}
	wg.Wait()
	return found, nil
}
