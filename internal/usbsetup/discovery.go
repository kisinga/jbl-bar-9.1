package usbsetup

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
)

func privateBarIP(ip netip.Addr) bool { return ip.Is4() && ip.IsPrivate() }

func resolveBar(ctx context.Context, host string) (string, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		if !privateBarIP(ip) {
			return "", errors.New("use the soundbar’s private home-network IPv4 address")
		}
		return ip.String(), nil
	}
	if !strings.HasSuffix(host, ".local") || len(host) > 253 || strings.ContainsAny(host, " /\\:@\t\r\n") {
		return "", errors.New("enter jbl.local or the soundbar’s private IPv4 address, without a port or URL")
	}
	// Use an OS resolver when it supports Bonjour, then native multicast DNS.
	osCtx, cancel := context.WithTimeout(ctx, time.Second)
	ips, _ := net.DefaultResolver.LookupNetIP(osCtx, "ip4", host)
	cancel()
	for _, ip := range ips {
		if privateBarIP(ip) {
			return ip.String(), nil
		}
	}
	return multicastBar(ctx, host)
}

func mdnsAnswer(data []byte, host string) string {
	var message dnsmessage.Message
	if message.Unpack(data) != nil || !message.Header.Response {
		return ""
	}
	for _, record := range append(message.Answers, message.Additionals...) {
		if !strings.EqualFold(record.Header.Name.String(), host+".") || record.Header.TTL == 0 {
			continue
		}
		if a, ok := record.Body.(*dnsmessage.AResource); ok {
			ip := netip.AddrFrom4(a.A)
			if privateBarIP(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

func multicastBar(ctx context.Context, host string) (string, error) {
	name, err := dnsmessage.NewName(host + ".")
	if err != nil {
		return "", errors.New("invalid soundbar name")
	}
	query, err := (&dnsmessage.Message{Questions: []dnsmessage.Question{{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET | 0x8000}}}).Pack()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	var sockets []*net.UDPConn
	var readers sync.WaitGroup
	found := make(chan string, 1)
	defer func() {
		for _, socket := range sockets {
			socket.Close()
		}
		readers.Wait()
	}()
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagMulticast != 0 && iface.Flags&net.FlagLoopback == 0 {
			// Share the mDNS port with Bonjour/Avahi. Some responders answer only
			// multicast, and host firewalls commonly allow 5353 rather than replies
			// to an arbitrary ephemeral port.
			socket, err := net.ListenMulticastUDP("udp4", &iface, group)
			if err != nil {
				continue
			}
			sockets = append(sockets, socket)
			packet := ipv4.NewPacketConn(socket)
			packet.SetMulticastTTL(255)
			packet.SetMulticastInterface(&iface)
			socket.SetReadDeadline(time.Now().Add(4 * time.Second))
			readers.Add(1)
			go func(socket *net.UDPConn) {
				defer readers.Done()
				buffer := make([]byte, 9000)
				for {
					n, _, err := socket.ReadFromUDP(buffer)
					if err != nil {
						return
					}
					if address := mdnsAnswer(buffer[:n], host); address != "" {
						select {
						case found <- address:
						default:
						}
						return
					}
				}
			}(socket)
			socket.WriteToUDP(query, group)
		}
	}
	select {
	case address := <-found:
		return address, nil
	case <-ctx.Done():
		return "", errors.New("couldn’t find the bar; keep it on the same network or enter its IP address")
	}
}
