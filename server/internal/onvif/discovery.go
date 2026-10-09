package onvif

import (
	"context"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Discovered struct {
	Address   string `json:"address"`    // IP address of the device
	DeviceURL string `json:"device_url"` // ONVIF device service URL
	Name      string `json:"name"`
	Hardware  string `json:"hardware"`
}

var multicastAddr = &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 3702}

// Discover sends a WS-Discovery probe to the local network and collects
// answers from ONVIF video devices until the timeout expires.
func Discover(ctx context.Context, timeout time.Duration) ([]Discovered, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.WriteToUDP([]byte(probeMessage()), multicastAddr); err != nil {
		return nil, fmt.Errorf("send probe: %w", err)
	}

	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetReadDeadline(deadline)

	found := map[string]Discovered{}
	buf := make([]byte, 64<<10)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break // deadline reached
		}
		for _, d := range parseProbeMatches(buf[:n], from.IP.String()) {
			found[d.DeviceURL] = d
		}
	}

	list := make([]Discovered, 0, len(found))
	for _, d := range found {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Address < list[j].Address })
	return list, nil
}

func probeMessage() string {
	id := make([]byte, 16)
	rand.Read(id)
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:])
	return `<?xml version="1.0" encoding="UTF-8"?>
<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
<e:Header><w:MessageID>uuid:` + uuid + `</w:MessageID><w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To><w:Action e:mustUnderstand="true">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action></e:Header>
<e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></e:Body></e:Envelope>`
}

func parseProbeMatches(data []byte, fromIP string) []Discovered {
	var env struct {
		Matches []struct {
			XAddrs string `xml:"XAddrs"`
			Scopes string `xml:"Scopes"`
		} `xml:"Body>ProbeMatches>ProbeMatch"`
	}
	if xml.Unmarshal(data, &env) != nil {
		return nil
	}
	var out []Discovered
	for _, m := range env.Matches {
		d := Discovered{Address: fromIP}
		// Prefer the address the answer came from: devices often list several.
		for _, addr := range strings.Fields(m.XAddrs) {
			u, err := url.Parse(addr)
			if err != nil {
				continue
			}
			if d.DeviceURL == "" || u.Hostname() == fromIP {
				d.DeviceURL = addr
			}
		}
		if d.DeviceURL == "" {
			continue
		}
		for _, scope := range strings.Fields(m.Scopes) {
			if v, ok := strings.CutPrefix(scope, "onvif://www.onvif.org/name/"); ok {
				d.Name, _ = url.PathUnescape(v)
			}
			if v, ok := strings.CutPrefix(scope, "onvif://www.onvif.org/hardware/"); ok {
				d.Hardware, _ = url.PathUnescape(v)
			}
		}
		out = append(out, d)
	}
	return out
}
