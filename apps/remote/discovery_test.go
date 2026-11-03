package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestDiscoveryRecordContainsOnlyRequestedAlias(t *testing.T) {
	b, e := discoveryRecordPayload(net.ParseIP("192.0.2.42"), 7)
	if e != nil {
		t.Fatal(e)
	}
	if b[0] != 0 || binary.BigEndian.Uint32(b[1:5]) != 0x20 || binary.BigEndian.Uint32(b[5:9]) != 0 || string(b[9:20]) != "jbl.local.\x00" {
		t.Fatalf("invalid native registration: %x", b)
	}
	if !bytes.Equal(b[20:26], []byte{0, 1, 0, 1, 0, 4}) || !bytes.Equal(b[26:30], []byte{192, 0, 2, 42}) || binary.BigEndian.Uint32(b[30:34]) != 120 {
		t.Fatalf("invalid A record: %x", b)
	}
	for _, ip := range []string{"::1", "0.0.0.0", "224.0.0.1", "255.255.255.255", "127.0.0.1"} {
		if _, e := discoveryRecordPayload(net.ParseIP(ip), 7); e == nil {
			t.Fatal("invalid advertised address accepted", ip)
		}
	}
	if _, e := discoveryRecordPayload(net.ParseIP("192.0.2.42"), 0); e == nil {
		t.Fatal("unbound interface accepted")
	}
}
func discoveryTestReply(code int32) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b[8:12], uint32(code))
	return discoveryPacket(69, 0, discoveryContext, b)
}
func TestDiscoveryCallbacksAndConflicts(t *testing.T) {
	if e := readDiscoveryReply(bytes.NewReader(discoveryTestReply(0))); e != nil {
		t.Fatal(e)
	}
	if e := readDiscoveryReply(bytes.NewReader(discoveryTestReply(-65548))); !errors.Is(e, errDiscoveryNameConflict) {
		t.Fatal("conflicting hostname was accepted", e)
	}
	if e := readDiscoveryReply(bytes.NewReader(discoveryTestReply(-65537))); e == nil {
		t.Fatal("daemon error was accepted")
	}
	for _, index := range []int{0, 12, 16} {
		b := discoveryTestReply(0)
		b[index] ^= 0x80
		if e := readDiscoveryReply(bytes.NewReader(b)); e == nil {
			t.Fatal("invalid callback header accepted", index)
		}
	}
	for _, size := range []uint32{0, 11, discoveryReplyLimit + 1, ^uint32(0)} {
		b := discoveryTestReply(0)
		binary.BigEndian.PutUint32(b[4:8], size)
		if e := readDiscoveryReply(bytes.NewReader(b)); e == nil {
			t.Fatal("invalid response size accepted", size)
		}
	}
	if e := readDiscoveryReply(bytes.NewReader(discoveryTestReply(0)[:30])); e == nil {
		t.Fatal("truncated callback accepted")
	}
}
func discoveryUnixPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	listener, e := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(t.TempDir(), "mdnsd"), Net: "unix"})
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	client, e := net.DialUnix("unix", nil, listener.Addr().(*net.UnixAddr))
	if e != nil {
		t.Fatal(e)
	}
	server, e := listener.AcceptUnix()
	if e != nil {
		client.Close()
		t.Fatal(e)
	}
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_ = server.SetDeadline(time.Now().Add(time.Second))
	return client, server
}
func TestDiscoveryNativeIPCAndWithdrawal(t *testing.T) {
	client, server := discoveryUnixPair(t)
	defer client.Close()
	defer server.Close()
	result := make(chan error, 1)
	payload, _ := discoveryRecordPayload(net.ParseIP("192.0.2.42"), 7)
	go func() {
		fail := func(e error) { result <- e }
		var h [discoveryHeaderSize]byte
		if _, e := io.ReadFull(server, h[:]); e != nil {
			fail(e)
			return
		}
		if !bytes.Equal(h[:], discoveryPacket(1, 0, 0, nil)) {
			fail(errors.New("expected native CreateConnection"))
			return
		}
		if e := discoveryWriteAll(server, []byte{0, 0, 0, 0}); e != nil {
			fail(e)
			return
		}
		if _, e := io.ReadFull(server, h[:]); e != nil {
			fail(e)
			return
		}
		if binary.BigEndian.Uint32(h[12:16]) != 2 || binary.BigEndian.Uint32(h[4:8]) != uint32(len(payload)) {
			fail(errors.New("expected bounded RegisterRecord"))
			return
		}
		b := make([]byte, len(payload))
		if _, e := io.ReadFull(server, b[:len(b)-1]); e != nil {
			fail(e)
			return
		}
		oob := make([]byte, syscall.CmsgSpace(4))
		n, oobn, _, _, e := server.ReadMsgUnix(b[len(b)-1:], oob)
		if e != nil || n != 1 || !bytes.Equal(b, payload) {
			fail(errors.New("invalid record byte/fd framing"))
			return
		}
		messages, e := syscall.ParseSocketControlMessage(oob[:oobn])
		if e != nil || len(messages) != 1 {
			fail(errors.New("missing separate error return socket"))
			return
		}
		fds, e := syscall.ParseUnixRights(&messages[0])
		if e != nil || len(fds) != 1 {
			fail(errors.New("wrong error return descriptors"))
			return
		}
		file := os.NewFile(uintptr(fds[0]), "received-error-socket")
		e = discoveryWriteAll(file, []byte{0, 0, 0, 0})
		_ = file.Close()
		if e != nil {
			fail(e)
			return
		}
		if e := discoveryWriteAll(server, discoveryTestReply(0)); e != nil {
			fail(e)
			return
		}
		var end [1]byte
		_, e = server.Read(end[:])
		if e != io.EOF {
			fail(errors.New("registration connection was not closed"))
			return
		}
		result <- nil
	}()
	r, e := registerDiscoveryConn(client, payload, time.Now().Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	if e := r.Close(); e != nil {
		t.Fatal("Close must be idempotent", e)
	}
	if e := <-result; e != nil {
		t.Fatal(e)
	}
}

func TestDiscoveryOnlyUsesReadyWiFiInAllowedLAN(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.0.2.0/24")
	iface := net.Interface{Name: "wlan0", Index: 7, Flags: net.FlagUp | net.FlagMulticast}
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("fe80::42"), Mask: net.CIDRMask(64, 128)},
		&net.IPNet{IP: net.ParseIP("198.51.100.9"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("192.0.2.42"), Mask: net.CIDRMask(24, 32)},
	}
	ip, index, e := discoveryInterfaceAddress(iface, addresses, network)
	if e != nil || !ip.Equal(net.ParseIP("192.0.2.42")) || index != 7 {
		t.Fatal(ip, index, e)
	}
	for _, flags := range []net.Flags{0, net.FlagUp, net.FlagMulticast} {
		bad := iface
		bad.Flags = flags
		if _, _, e := discoveryInterfaceAddress(bad, addresses, network); e == nil {
			t.Fatal("inactive/nonmulticast interface accepted", flags)
		}
	}
	if _, _, e := discoveryInterfaceAddress(iface, addresses[:2], network); e == nil {
		t.Fatal("address outside configured LAN accepted")
	}
	if _, _, e := discoveryInterfaceAddress(iface, addresses, nil); e == nil {
		t.Fatal("unrestricted discovery accepted")
	}
}
func TestDiscoveryHostnameOnlyPublishesConfirmedValue(t *testing.T) {
	defer setDiscoveryHostname("")
	setDiscoveryHostname("")
	if discoveryHostname() != "" {
		t.Fatal("unconfirmed hostname advertised")
	}
	setDiscoveryHostname("jbl.local")
	if discoveryHostname() != "jbl.local" {
		t.Fatal("confirmed hostname missing")
	}
	setDiscoveryHostname("")
	if discoveryHostname() != "" {
		t.Fatal("withdrawn hostname remained visible")
	}
}
