package discovery

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/gepfrad/gepfradprint/internal/model"
)

type record struct {
	name    string
	typ     uint16
	data    []byte
	packet  []byte
	dataOff int
}

var services = []string{"_ipp._tcp.local", "_ipps._tcp.local", "_printer._tcp.local"}

func Discover(ctx context.Context) ([]model.Printer, error) {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer c.Close()
	dst := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	for _, svc := range services {
		_, _ = c.WriteToUDP(dnsQuery(svc, uint16(rand.Intn(65535))), dst)
	}
	deadline := time.Now().Add(3 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.SetReadDeadline(deadline)

	var all []record
	buf := make([]byte, 65535)
	for {
		n, _, e := c.ReadFromUDP(buf)
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Timeout() {
				break
			}
			continue
		}
		all = append(all, parseDNS(buf[:n])...)
	}
	return assemble(all), nil
}

func assemble(records []record) []model.Printer {
	ptrs := map[string][]string{}
	txts := map[string]map[string]string{}
	srvs := map[string]struct {
		host string
		port int
	}{}
	addresses := map[string]string{}

	for _, r := range records {
		switch r.typ {
		case 12: // PTR
			if target, _ := readName(r.packet, r.dataOff); target != "" {
				ptrs[r.name] = append(ptrs[r.name], target)
			}
		case 33: // SRV
			if len(r.data) >= 6 {
				target, _ := readName(r.packet, r.dataOff+6)
				if target != "" {
					srvs[strings.TrimSuffix(r.name, ".")] = struct {
						host string
						port int
					}{host: strings.TrimSuffix(target, "."), port: int(binary.BigEndian.Uint16(r.data[4:6]))}
				}
			}
		case 16: // TXT
			if m := parseTXT(r.data); m != nil {
				txts[strings.TrimSuffix(r.name, ".")] = m
			}
		case 1: // A
			if len(r.data) == 4 {
				addresses[strings.TrimSuffix(r.name, ".")] = net.IP(r.data).String()
			}
		}
	}

	seen := map[string]model.Printer{}
	for serviceType, instances := range ptrs {
		if !isPrinterService(serviceType) {
			continue
		}
		for _, instance := range instances {
			srv, ok := srvs[strings.TrimSuffix(instance, ".")]
			if !ok {
				continue
			}
			attrs := txts[strings.TrimSuffix(instance, ".")]
			proto := "ipp"
			scheme := "http"
			if strings.Contains(strings.ToLower(serviceType), "_ipps") {
				proto = "ipps"
				scheme = "https"
			} else if strings.Contains(strings.ToLower(serviceType), "_printer._tcp") {
				proto = "lpr"
			}
			path := attrs["rp"]
			if path == "" {
				path = "ipp/print"
			}
			path = "/" + strings.TrimPrefix(path, "/")
			host := srv.host
			address := addresses[host]
			name := instance
			if typ := attrs["ty"]; typ != "" {
				name = typ
			} else if idx := strings.Index(instance, "._"); idx > 0 {
				name = instance[:idx]
			}
			uriHost := host
			uri := ""
			if proto == "lpr" {
				uri = fmt.Sprintf("lpr://%s:%d/%s", uriHost, srv.port, strings.TrimPrefix(path, "/"))
			} else {
				uri = fmt.Sprintf("%s://%s:%d%s", scheme, uriHost, srv.port, path)
			}
			id := fmt.Sprintf("%s:%d/%s", host, srv.port, proto)
			if existing, ok := seen[id]; ok {
				if existing.Protocol == "ipp" && proto == "ipps" {
					continue
				}
				continue
			}
			p := model.Printer{
				ID:           id,
				Name:         name,
				Host:         host,
				Address:      address,
				Port:         srv.port,
				URI:          uri,
				Protocol:     proto,
				RP:           path,
				DiscoveredAt: time.Now(),
				TXT:          attrs,
			}
			seen[id] = p
		}
	}

	out := make([]model.Printer, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func isPrinterService(s string) bool {
	s = strings.ToLower(strings.TrimSuffix(s, "."))
	return s == "_ipp._tcp.local" || s == "_ipps._tcp.local" || s == "_printer._tcp.local"
}

func parseTXT(b []byte) map[string]string {
	if len(b) == 0 {
		return nil
	}
	out := map[string]string{}
	for i := 0; i < len(b); {
		l := int(b[i])
		i++
		if i+l > len(b) {
			break
		}
		item := string(b[i : i+l])
		i += l
		if eq := strings.IndexByte(item, '='); eq >= 0 {
			out[item[:eq]] = item[eq+1:]
		} else if item != "" {
			out[item] = ""
		}
	}
	return out
}

func dnsQuery(name string, id uint16) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b, id)
	binary.BigEndian.PutUint16(b[4:], 1)
	for _, s := range strings.Split(name, ".") {
		b = append(b, byte(len(s)))
		b = append(b, []byte(s)...)
	}
	b = append(b, 0, 0, 12, 0, 1)
	return b
}

func parseDNS(b []byte) []record {
	if len(b) < 12 {
		return nil
	}
	qd := int(binary.BigEndian.Uint16(b[4:6]))
	counts := []int{int(binary.BigEndian.Uint16(b[6:8])), int(binary.BigEndian.Uint16(b[8:10])), int(binary.BigEndian.Uint16(b[10:12]))}
	i := 12
	for n := 0; n < qd; n++ {
		_, i = readName(b, i)
		if i+4 > len(b) {
			return nil
		}
		i += 4
	}
	var out []record
	for _, count := range counts {
		for n := 0; n < count; n++ {
			name, ni := readName(b, i)
			i = ni
			if i+10 > len(b) {
				return out
			}
			typ := binary.BigEndian.Uint16(b[i : i+2])
			l := int(binary.BigEndian.Uint16(b[i+8 : i+10]))
			i += 10
			if i+l > len(b) {
				return out
			}
			out = append(out, record{name: name, typ: typ, data: append([]byte(nil), b[i:i+l]...), packet: b, dataOff: i})
			i += l
		}
	}
	return out
}

func readName(b []byte, i int) (string, int) {
	var parts []string
	start := i
	visited := map[int]bool{}
	for i < len(b) {
		if visited[i] {
			return strings.Join(parts, "."), i + 1
		}
		visited[i] = true
		l := int(b[i])
		if l == 0 {
			return strings.Join(parts, "."), i + 1
		}
		if l&0xc0 == 0xc0 {
			if i+1 >= len(b) {
				return strings.Join(parts, "."), i + 2
			}
			ptr := int(binary.BigEndian.Uint16(b[i:i+2]) & 0x3fff)
			s, _ := readName(b, ptr)
			if s != "" {
				parts = append(parts, s)
			}
			return strings.Join(parts, "."), i + 2
		}
		i++
		if i+l > len(b) {
			return strings.Join(parts, "."), len(b)
		}
		parts = append(parts, string(b[i:i+l]))
		i += l
	}
	if i == start {
		return "", i
	}
	return strings.Join(parts, "."), i
}
