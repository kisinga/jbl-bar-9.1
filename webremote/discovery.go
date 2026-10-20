package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// The soundbar already runs Apple's Bonjour daemon. Register an additional A
// record through its DNS-SD IPC API rather than running a second mDNS responder.
// Protocol reference: Apple mDNSResponder mDNSShared/dnssd_ipc.h and
// dnssd_clientstub.c (DNSServiceCreateConnection / DNSServiceRegisterRecord).
const (
	discoveryName       = "jbl.local."
	discoverySocket     = "/var/run/mdnsd"
	discoveryHeaderSize = 28
	discoveryReplyLimit = 2048
	discoveryContext    = uint64(1) // Opaque cookie echoed by the daemon.
)

var errDiscoveryNameConflict = errors.New("jbl.local is already in use on this network")

type discoveryRegistration struct {
	conn   *net.UnixConn
	done   chan error
	closed atomic.Bool
	once   sync.Once
}

// Done closes when the registration ends. An unexpected daemon/protocol error
// is sent once before closing; intentional Close/context cancellation is quiet.
func (r *discoveryRegistration) Done() <-chan error { return r.done }

// Closing the DNS-SD connection withdraws the alias; Bonjour sends its goodbye.
func (r *discoveryRegistration) Close() error {
	var e error
	r.once.Do(func() {
		r.closed.Store(true)
		e = r.conn.Close()
	})
	return e
}

func discoveryError(code int32) error {
	if code == 0 {
		return nil
	}
	if code == -65548 {
		return errDiscoveryNameConflict
	}
	return fmt.Errorf("Bonjour registration error %d", code)
}
func discoveryPacket(op, index uint32, context uint64, payload []byte) []byte {
	b := make([]byte, discoveryHeaderSize, discoveryHeaderSize+len(payload))
	binary.BigEndian.PutUint32(b[0:4], 1)
	binary.BigEndian.PutUint32(b[4:8], uint32(len(payload)))
	binary.BigEndian.PutUint32(b[12:16], op)
	// client_context is opaque in Apple's protocol, not a converted integer.
	// Its byte order does not matter as long as the reply echoes these bytes.
	binary.BigEndian.PutUint64(b[16:24], context)
	binary.BigEndian.PutUint32(b[24:28], index)
	return append(b, payload...)
}
func discoveryRecordPayload(ip net.IP, interfaceIndex uint32) ([]byte, error) {
	v4 := ip.To4()
	if v4 == nil || v4.IsUnspecified() || v4.IsMulticast() || !v4.IsGlobalUnicast() || interfaceIndex == 0 {
		return nil, errors.New("Bonjour needs a local IPv4 address and interface")
	}
	// An empty error-return pathname precedes DNSServiceRegisterRecord data.
	b := make([]byte, 1+8)
	binary.BigEndian.PutUint32(b[1:5], 0x20) // kDNSServiceFlagsUnique; daemon probes/conflict-checks.
	// Bonjour's interface objects can change during Wi-Fi/IPv6 refreshes even
	// while Linux keeps the same wlan0 index. A record bound to that specific
	// native interface stops answering after such a refresh. DNS-SD Any follows
	// the daemon's current interfaces; the monitor still selects and withdraws
	// this address using the allowed wlan0 interface and configured LAN.
	binary.BigEndian.PutUint32(b[5:9], 0) // kDNSServiceInterfaceIndexAny.
	b = append(b, discoveryName...)
	b = append(b, 0)
	b = binary.BigEndian.AppendUint16(b, 1) // A.
	b = binary.BigEndian.AppendUint16(b, 1) // IN.
	b = binary.BigEndian.AppendUint16(b, 4)
	b = append(b, v4...)
	b = binary.BigEndian.AppendUint32(b, 120) // TTL in seconds.
	return b, nil
}
func discoveryWriteAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func readDiscoveryError(r io.Reader) error {
	var b [4]byte
	if _, e := io.ReadFull(r, b[:]); e != nil {
		return e
	}
	return discoveryError(int32(binary.BigEndian.Uint32(b[:])))
}

// RegisterRecord uses a separate socket for its immediate error result so it
// cannot be confused with asynchronous record callbacks on the shared socket.
// Follow Apple's client: write all but the final payload byte, then send that
// last byte together with SCM_RIGHTS for one end of a fresh socketpair.
func sendDiscoveryRecord(conn *net.UnixConn, packet []byte, deadline time.Time) error {
	pair, e := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if e != nil {
		return e
	}
	readerFile := os.NewFile(uintptr(pair[0]), "bonjour-error-reader")
	writerFile := os.NewFile(uintptr(pair[1]), "bonjour-error-writer")
	defer readerFile.Close()
	defer writerFile.Close()
	reader, e := net.FileConn(readerFile)
	if e != nil {
		return e
	}
	defer reader.Close()
	if e := reader.SetDeadline(deadline); e != nil {
		return e
	}
	if len(packet) <= discoveryHeaderSize {
		return errors.New("empty Bonjour registration")
	}
	if e := discoveryWriteAll(conn, packet[:len(packet)-1]); e != nil {
		return e
	}
	n, _, e := conn.WriteMsgUnix(packet[len(packet)-1:], syscall.UnixRights(int(writerFile.Fd())), nil)
	if e != nil {
		return e
	}
	if n != 1 {
		return io.ErrShortWrite
	}
	return readDiscoveryError(reader)
}
func readDiscoveryReply(r io.Reader) error {
	var h [discoveryHeaderSize]byte
	if _, e := io.ReadFull(r, h[:]); e != nil {
		return e
	}
	size := binary.BigEndian.Uint32(h[4:8])
	if binary.BigEndian.Uint32(h[:4]) != 1 || size < 12 || size > discoveryReplyLimit || binary.BigEndian.Uint32(h[12:16]) != 69 || binary.BigEndian.Uint64(h[16:24]) != discoveryContext {
		return errors.New("unexpected Bonjour registration response")
	}
	b := make([]byte, size)
	if _, e := io.ReadFull(r, b); e != nil {
		return e
	}
	return discoveryError(int32(binary.BigEndian.Uint32(b[8:12])))
}

// registerDiscovery waits for native probing to succeed. Run it in a background
// monitor, retry daemon unavailability with a delay, and recreate it when the
// selected interface/address changes. A name conflict must stay visible rather
// than silently assigning the requested name to a different device.
func registerDiscovery(parent context.Context, ip net.IP, interfaceIndex uint32) (*discoveryRegistration, error) {
	payload, e := discoveryRecordPayload(ip, interfaceIndex)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	c, e := (&net.Dialer{}).DialContext(ctx, "unix", discoverySocket)
	if e != nil {
		return nil, fmt.Errorf("Bonjour daemon unavailable: %w", e)
	}
	conn := c.(*net.UnixConn)
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()
	deadline, _ := ctx.Deadline()
	if e = conn.SetDeadline(deadline); e != nil {
		_ = conn.Close()
		return nil, e
	}
	r, e := registerDiscoveryConn(conn, payload, deadline)
	if e != nil {
		_ = conn.Close()
		return nil, e
	}
	if !stopCancellation() && ctx.Err() != nil {
		_ = r.Close()
		return nil, ctx.Err()
	}
	stopLifetime := context.AfterFunc(parent, func() { _ = r.Close() })
	go func() {
		defer close(r.done)
		defer stopLifetime()
		defer r.Close()
		for {
			if e := readDiscoveryReply(conn); e != nil {
				if !r.closed.Load() {
					r.done <- e
				}
				return
			}
		}
	}()
	return r, nil
}
func registerDiscoveryConn(conn *net.UnixConn, payload []byte, deadline time.Time) (*discoveryRegistration, error) {
	if e := discoveryWriteAll(conn, discoveryPacket(1, 0, 0, nil)); e != nil {
		return nil, e
	}
	if e := readDiscoveryError(conn); e != nil {
		return nil, e
	}
	if e := sendDiscoveryRecord(conn, discoveryPacket(2, 0, discoveryContext, payload), deadline); e != nil {
		return nil, e
	}
	if e := readDiscoveryReply(conn); e != nil {
		return nil, e
	}
	if e := conn.SetDeadline(time.Time{}); e != nil {
		return nil, e
	}
	return &discoveryRegistration{conn: conn, done: make(chan error, 1)}, nil
}

var discoveryStatus struct {
	sync.RWMutex
	hostname string
}

// discoveryHostname advertises a URL only after Bonjour confirms registration.
func discoveryHostname() string {
	discoveryStatus.RLock()
	defer discoveryStatus.RUnlock()
	return discoveryStatus.hostname
}
func setDiscoveryHostname(hostname string) {
	discoveryStatus.Lock()
	discoveryStatus.hostname = hostname
	discoveryStatus.Unlock()
}
func discoveryInterfaceAddress(iface net.Interface, addresses []net.Addr, network *net.IPNet) (net.IP, uint32, error) {
	if network == nil || iface.Index <= 0 || iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
		return nil, 0, errors.New("Wi-Fi multicast is not ready")
	}
	for _, address := range addresses {
		var ip net.IP
		switch address := address.(type) {
		case *net.IPNet:
			ip = address.IP.To4()
		case *net.IPAddr:
			ip = address.IP.To4()
		}
		if ip != nil && ip.IsGlobalUnicast() && network.Contains(ip) {
			return append(net.IP(nil), ip...), uint32(iface.Index), nil
		}
	}
	return nil, 0, errors.New("Wi-Fi has no address in the configured LAN")
}
func discoveryWiFiAddress(network *net.IPNet) (net.IP, uint32, error) {
	iface, e := net.InterfaceByName("wlan0")
	if e != nil {
		return nil, 0, errors.New("Wi-Fi interface is not available")
	}
	addresses, e := iface.Addrs()
	if e != nil {
		return nil, 0, errors.New("Wi-Fi addresses are not available")
	}
	return discoveryInterfaceAddress(*iface, addresses, network)
}

// monitorDiscovery follows the existing Wi-Fi address without changing it.
// It owns only this process's alias registration; the system Bonjour daemon
// remains responsible for all existing AirPlay/Cast advertisements.
func monitorDiscovery(ctx context.Context, network *net.IPNet) {
	var registration *discoveryRegistration
	var registeredIP net.IP
	var registeredIndex uint32
	var nextAttempt time.Time
	var desired, lastMessage string
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer func() {
		setDiscoveryHostname("")
		if registration != nil {
			_ = registration.Close()
		}
	}()
	report := func(message string) {
		if message != lastMessage {
			log.Printf("Local address: %s", message)
			lastMessage = message
		}
	}
	withdraw := func() {
		setDiscoveryHostname("")
		if registration != nil {
			_ = registration.Close()
			registration = nil
		}
		registeredIP, registeredIndex = nil, 0
	}
	for {
		if ctx.Err() != nil {
			return
		}
		ip, index, addressErr := discoveryWiFiAddress(network)
		currentDesired := ""
		if addressErr == nil {
			currentDesired = fmt.Sprintf("%d/%s", index, ip)
		}
		if currentDesired != desired {
			desired = currentDesired
			nextAttempt = time.Time{}
		}
		if registration != nil && (addressErr != nil || index != registeredIndex || !ip.Equal(registeredIP)) {
			withdraw()
		}
		if addressErr != nil {
			setDiscoveryHostname("")
			report(addressErr.Error())
		} else if registration == nil && !time.Now().Before(nextAttempt) {
			r, e := registerDiscovery(ctx, ip, index)
			if ctx.Err() != nil {
				if r != nil {
					_ = r.Close()
				}
				return
			}
			if e != nil {
				setDiscoveryHostname("")
				nextAttempt = time.Now().Add(30 * time.Second)
				report(e.Error())
			} else {
				registration, registeredIP, registeredIndex = r, ip, index
				setDiscoveryHostname("jbl.local")
				report("jbl.local is ready")
			}
		}
		var ended <-chan error
		if registration != nil {
			ended = registration.Done()
		}
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ended:
			withdraw()
			if ctx.Err() != nil {
				return
			}
			nextAttempt = time.Now().Add(30 * time.Second)
			if ok && e != nil {
				report(e.Error())
			} else {
				report("Bonjour registration ended; retrying")
			}
		case <-ticker.C:
		}
	}
}
