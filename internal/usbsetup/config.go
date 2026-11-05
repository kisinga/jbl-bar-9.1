package usbsetup

import (
	"net"
	"net/netip"
	"strconv"
)

func validNetwork(s string) bool {
	p, e := netip.ParsePrefix(s)
	return e == nil && p.Addr().Is4() && p.Addr().IsPrivate() && p.Bits() >= 16 && p.Bits() <= 30 && p == p.Masked() && s == p.String()
}
func validPort(s string) bool {
	v, e := strconv.Atoi(s)
	return e == nil && strconv.Itoa(v) == s && (v == 80 || v >= 1024 && v <= 65535 && v != 2222)
}

type Network struct {
	Address string `json:"address"`
	Prefix  int    `json:"prefix"`
}

func Networks() []Network {
	out := []Network{}
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := i.Addrs()
		for _, a := range addresses {
			ip, n, e := net.ParseCIDR(a.String())
			if e == nil && ip.To4() != nil && ip.IsPrivate() {
				ones, _ := n.Mask.Size()
				if ones >= 16 && ones <= 30 {
					out = append(out, Network{ip.String(), ones})
				}
			}
		}
	}
	return out
}
