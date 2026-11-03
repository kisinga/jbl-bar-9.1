package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"os"
	"strings"
	"unicode/utf8"
)

func peerAddress(b []byte) string {
	n := bytes.IndexByte(b, 0)
	if n != 17 {
		return ""
	}
	s := string(b[:n])
	mac, e := net.ParseMAC(s)
	if e != nil || len(mac) != 6 {
		return ""
	}
	return mac.String()
}
func peerName(config []byte, address string) string {
	// bt_config also contains pairing keys. Read only Name from the exact peer section.
	if address == "" || len(config) > 128*1024 {
		return ""
	}
	wanted := false
	result := ""
	scan := bufio.NewScanner(bytes.NewReader(config))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			wanted = strings.EqualFold(line[1:len(line)-1], address)
			continue
		}
		if !wanted {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) != "Name" {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) == 0 || len(v) > 248 || !utf8.ValidString(v) {
			return ""
		}
		for _, r := range v {
			if r < 32 || r == 127 {
				return ""
			}
		}
		result = v
	}
	if scan.Err() != nil {
		return ""
	}
	return result
}
func readPeerName(address string) string {
	if address == "" {
		return ""
	}
	f, e := os.Open("/data/misc/bluedroid/bt_config.conf")
	if e != nil {
		return ""
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 128*1024+1))
	if e != nil {
		return ""
	}
	return peerName(b, address)
}
