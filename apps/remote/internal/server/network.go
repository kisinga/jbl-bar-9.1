package server

import "net"

func AllowedNetwork(client, host, local net.IP, lan *net.IPNet, ap *net.IPNet) bool {
	if client == nil || host == nil {
		return false
	}
	if lan.Contains(client) && lan.Contains(host) {
		return true
	}
	return ap != nil && host.Equal(ap.IP) && local != nil && local.Equal(ap.IP) && ap.Contains(client) && !client.Equal(ap.IP)
}
