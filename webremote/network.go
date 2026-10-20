package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type wifiState struct {
	HotspotIP string `json:"hotspot_ip,omitempty"`
	Available bool   `json:"available"`
	Connected bool   `json:"connected"`
	SSID      string `json:"ssid,omitempty"`
	IP        string `json:"ip,omitempty"`
	Signal    *int   `json:"signal_dbm,omitempty"`
	Frequency *int   `json:"frequency_mhz,omitempty"`
}
type networkCache struct {
	sync.Mutex
	state wifiState
}

var wifiCache networkCache

func wpaRequest(endpoint, command string) (string, error) {
	nonce := make([]byte, 12)
	if _, e := rand.Read(nonce); e != nil {
		return "", e
	}
	// Abstract client sockets let Android's Wi-Fi UID reply without granting it
	// access to a root-owned temporary directory. connect() pins the peer socket.
	c, e := net.DialUnix("unixgram", &net.UnixAddr{Name: "@jbl-wifi-" + hex.EncodeToString(nonce), Net: "unixgram"}, &net.UnixAddr{Name: endpoint, Net: "unixgram"})
	if e != nil {
		return "", e
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(400 * time.Millisecond))
	if _, e = c.Write([]byte(command)); e != nil {
		return "", e
	}
	b := make([]byte, 16384)
	n, e := c.Read(b)
	if e != nil {
		return "", e
	}
	if n == len(b) {
		return "", errors.New("oversized network response")
	}
	return string(b[:n]), nil
}
func keyValues(s string) map[string]string {
	values := map[string]string{}
	scan := bufio.NewScanner(strings.NewReader(s))
	for scan.Scan() {
		if k, v, ok := strings.Cut(scan.Text(), "="); ok {
			values[k] = v
		}
	}
	return values
}
func ssidText(s string) string {
	// Supplicant STATUS uses printf-style escaping, not a quoted shell argument.
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'x':
				if i+2 >= len(s) {
					return ""
				}
				v, e := hex.DecodeString(s[i+1 : i+3])
				if e != nil {
					return ""
				}
				b = append(b, v[0])
				i += 2
			case '\\', '"':
				b = append(b, s[i])
			case 't':
				b = append(b, '\t')
			default:
				return ""
			}
		} else {
			b = append(b, s[i])
		}
	}
	if len(b) > 32 || !utf8.Valid(b) {
		return ""
	}
	return string(b)
}
func parseWiFi(status, signal string) wifiState {
	v := keyValues(status)
	s := wifiState{Available: v["wpa_state"] != "", Connected: v["wpa_state"] == "COMPLETED"}
	if s.Connected {
		s.SSID = ssidText(v["ssid"])
		if ip := net.ParseIP(v["ip_address"]); ip != nil {
			s.IP = ip.String()
		}
	}
	p := keyValues(signal)
	if n, e := strconv.Atoi(p["RSSI"]); s.Connected && e == nil && n >= -127 && n <= 0 {
		s.Signal = &n
	}
	if n, e := strconv.Atoi(p["FREQUENCY"]); s.Connected && e == nil && n >= 2300 && n <= 7125 {
		s.Frequency = &n
	}
	return s
}
func hotspotAddress() net.IP {
	iface, e := net.InterfaceByName("ap0")
	if e != nil || iface.Flags&net.FlagUp == 0 {
		return nil
	}
	addresses, e := iface.Addrs()
	if e != nil {
		return nil
	}
	for _, address := range addresses {
		if p, ok := address.(*net.IPNet); ok && p.IP.Equal(net.ParseIP("192.168.255.249")) {
			return p.IP
		}
	}
	return nil
}
func readWiFi() wifiState { wifiCache.Lock(); defer wifiCache.Unlock(); return wifiCache.state }
func refreshWiFi() {
	result := wifiState{}
	if ip := hotspotAddress(); ip != nil {
		result.HotspotIP = ip.String()
	}
	for _, endpoint := range []string{"/dev/socket/wpa_wlan0", "/data/wifi/wlan0", "/var/run/wpa_supplicant/wlan0"} {
		if st, e := os.Stat(endpoint); e != nil || st.Mode()&os.ModeSocket == 0 {
			continue
		}
		prefix := "IFNAME=wlan0 "
		status, e := wpaRequest(endpoint, prefix+"STATUS")
		if e != nil || !strings.Contains(status, "wpa_state=") {
			prefix = ""
			status, e = wpaRequest(endpoint, "STATUS")
		}
		if e != nil || !strings.Contains(status, "wpa_state=") {
			continue
		}
		signal, _ := wpaRequest(endpoint, prefix+"SIGNAL_POLL")
		parsed := parseWiFi(status, signal)
		parsed.HotspotIP = result.HotspotIP
		result = parsed
		break
	}
	if result.Connected && result.IP == "" {
		if iface, e := net.InterfaceByName("wlan0"); e == nil {
			if addresses, e := iface.Addrs(); e == nil {
				for _, address := range addresses {
					if p, ok := address.(*net.IPNet); ok && p.IP.To4() != nil {
						result.IP = p.IP.String()
						break
					}
				}
			}
		}
	}
	wifiCache.Lock()
	wifiCache.state = result
	wifiCache.Unlock()
}
func monitorWiFi(ctx context.Context) {
	refreshWiFi()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshWiFi()
		}
	}
}
func allowedNetwork(client, host, local net.IP, lan *net.IPNet, ap net.IP) bool {
	if client == nil || host == nil {
		return false
	}
	if lan.Contains(client) && lan.Contains(host) {
		return true
	}
	_, subnet, _ := net.ParseCIDR("192.168.255.248/29")
	return ap != nil && host.Equal(ap) && local != nil && local.Equal(ap) && subnet.Contains(client) && !client.Equal(ap)
}
