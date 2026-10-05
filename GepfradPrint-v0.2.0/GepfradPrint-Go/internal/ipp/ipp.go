package ipp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gepfrad/gepfradprint/internal/model"
)

const (
	VersionMajor           byte   = 2
	VersionMinor           byte   = 0
	OpPrintJob             uint16 = 0x0002
	OpGetPrinterAttributes uint16 = 0x000B
	TagOperation           byte   = 0x01
	TagJob                 byte   = 0x02
	TagEnd                 byte   = 0x03
	TagInteger             byte   = 0x21
	TagBoolean             byte   = 0x22
	TagEnum                byte   = 0x23
	TagText                byte   = 0x41
	TagName                byte   = 0x42
	TagKeyword             byte   = 0x44
	TagURI                 byte   = 0x45
	TagCharset             byte   = 0x47
	TagLanguage            byte   = 0x48
	TagMime                byte   = 0x49
)

type Attr struct {
	Tag   byte
	Name  string
	Value any
}

type Response struct {
	Status     uint16
	Attributes []Attr
}

type Endpoint struct {
	URI     string
	Host    string
	Address string
	Port    int
}

func FromPrinter(p model.Printer) Endpoint {
	return Endpoint{URI: p.URI, Host: p.Host, Address: p.Address, Port: p.Port}
}

func encString(buf *bytes.Buffer, tag byte, name, value string) {
	buf.WriteByte(tag)
	_ = binary.Write(buf, binary.BigEndian, uint16(len(name)))
	buf.WriteString(name)
	_ = binary.Write(buf, binary.BigEndian, uint16(len(value)))
	buf.WriteString(value)
}

func encAdditionalString(buf *bytes.Buffer, tag byte, value string) {
	buf.WriteByte(tag)
	_ = binary.Write(buf, binary.BigEndian, uint16(0))
	_ = binary.Write(buf, binary.BigEndian, uint16(len(value)))
	buf.WriteString(value)
}

func encInt(buf *bytes.Buffer, tag byte, name string, v int32) {
	buf.WriteByte(tag)
	_ = binary.Write(buf, binary.BigEndian, uint16(len(name)))
	buf.WriteString(name)
	_ = binary.Write(buf, binary.BigEndian, v)
}

func header(op uint16, id uint32) *bytes.Buffer {
	b := new(bytes.Buffer)
	b.Write([]byte{VersionMajor, VersionMinor})
	_ = binary.Write(b, binary.BigEndian, op)
	_ = binary.Write(b, binary.BigEndian, id)
	return b
}

func GetPrinterAttributes(ctx context.Context, uri string) (Response, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Response{}, fmt.Errorf("invalid printer URI: %w", err)
	}
	p := Endpoint{URI: uri, Host: u.Hostname(), Port: defaultPort(u.Scheme, u.Port())}
	return GetPrinterAttributesEndpoint(ctx, p)
}

func GetPrinterAttributesEndpoint(ctx context.Context, p Endpoint) (Response, error) {
	if p.URI == "" {
		return Response{}, errors.New("printer URI is empty")
	}
	b := header(OpGetPrinterAttributes, requestID())
	b.WriteByte(TagOperation)
	encString(b, TagCharset, "attributes-charset", "utf-8")
	encString(b, TagLanguage, "attributes-natural-language", "en")
	encString(b, TagURI, "printer-uri", p.URI)
	encString(b, TagKeyword, "requested-attributes", "all")
	resp, err := doEndpoint(ctx, p, b.Bytes())
	if err != nil {
		return resp, err
	}
	if resp.Status != 0x0000 {
		return resp, fmt.Errorf("IPP Get-Printer-Attributes failed: %s", FormatStatus(resp.Status, resp))
	}
	return resp, nil
}

func PrintJob(ctx context.Context, uri, name, mediaType string, data []byte, copies int, media, orientation string, color, duplex bool) (Response, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Response{}, fmt.Errorf("invalid printer URI: %w", err)
	}
	p := Endpoint{URI: uri, Host: u.Hostname(), Port: defaultPort(u.Scheme, u.Port())}
	return PrintJobEndpoint(ctx, p, name, mediaType, data, copies, media, orientation, color, duplex)
}

func PrintJobEndpoint(ctx context.Context, p Endpoint, name, mediaType string, data []byte, copies int, media, orientation string, color, duplex bool) (Response, error) {
	if copies < 1 {
		copies = 1
	}
	b := header(OpPrintJob, requestID())
	b.WriteByte(TagOperation)
	encString(b, TagCharset, "attributes-charset", "utf-8")
	encString(b, TagLanguage, "attributes-natural-language", "en")
	encString(b, TagURI, "printer-uri", p.URI)
	encString(b, TagName, "requesting-user-name", "Gepfrad Print")
	encString(b, TagName, "job-name", name)
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	encString(b, TagMime, "document-format", mediaType)
	encInt(b, TagInteger, "copies", int32(copies))
	if media != "" {
		encString(b, TagKeyword, "media", media)
	}
	if n, err := strconv.Atoi(orientation); err == nil && (n == 3 || n == 4) {
		encInt(b, TagEnum, "orientation-requested", int32(n))
	}
	if color {
		encString(b, TagKeyword, "print-color-mode", "color")
	} else {
		encString(b, TagKeyword, "print-color-mode", "monochrome")
	}
	if duplex {
		encString(b, TagKeyword, "sides", "two-sided-long-edge")
	}
	b.WriteByte(TagEnd)
	b.Write(data)

	resp, err := doEndpoint(ctx, p, b.Bytes())
	if err != nil {
		return resp, err
	}
	if resp.Status != 0x0000 {
		return resp, fmt.Errorf("IPP Print-Job rejected: %s", FormatStatus(resp.Status, resp))
	}
	return resp, nil
}

func TestConnection(ctx context.Context, p Endpoint) (Diagnostics, error) {
	d := Diagnostics{}
	host := p.Host
	if host == "" {
		host = hostFromURI(p.URI)
	}
	port := p.Port
	if port == 0 {
		port = 631
	}
	address := p.Address
	if address == "" {
		ips, err := net.LookupIP(host)
		if err != nil {
			d.DNS = Step{OK: false, Detail: err.Error()}
			return d, nil
		}
		if len(ips) == 0 {
			d.DNS = Step{OK: false, Detail: "no addresses returned"}
			return d, nil
		}
		address = ips[0].String()
	}
	d.DNS = Step{OK: true, Detail: address}

	dialer := &net.Dialer{Timeout: 6 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		d.TCP = Step{OK: false, Detail: err.Error()}
		return d, nil
	}
	d.TCP = Step{OK: true, Detail: net.JoinHostPort(address, strconv.Itoa(port))}

	if strings.EqualFold(schemeFromURI(p.URI), "https") {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}) // local printers often use an untrusted certificate
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			d.TLS = Step{OK: false, Detail: err.Error()}
			return d, nil
		}
		state := tlsConn.ConnectionState()
		d.TLS = Step{OK: true, Detail: fmt.Sprintf("TLS %s; certificate verification skipped for local printer", tlsVersion(state.Version))}
		_ = tlsConn.Close()
	} else {
		d.TLS = Step{OK: true, Detail: "not required for plain IPP"}
		_ = conn.Close()
	}

	resp, err := GetPrinterAttributesEndpoint(ctx, p)
	if err != nil {
		d.IPP = Step{OK: false, Detail: err.Error()}
		return d, nil
	}
	d.IPP = Step{OK: true, Detail: fmt.Sprintf("status=0x%04x", resp.Status)}
	d.PrinterState = attrString(resp, "printer-state")
	d.DocumentFormats = attrStrings(resp, "document-format-supported")
	d.Media = attrStrings(resp, "media-supported")
	return d, nil
}

type Step struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type Diagnostics struct {
	DNS             Step     `json:"dns"`
	TCP             Step     `json:"tcp"`
	TLS             Step     `json:"tls"`
	IPP             Step     `json:"ipp"`
	PrinterState    string   `json:"printer_state,omitempty"`
	DocumentFormats []string `json:"document_formats,omitempty"`
	Media           []string `json:"media,omitempty"`
}

func doEndpoint(ctx context.Context, p Endpoint, payload []byte) (Response, error) {
	u, err := url.Parse(p.URI)
	if err != nil {
		return Response{}, fmt.Errorf("invalid printer URI: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Response{}, fmt.Errorf("unsupported IPP URI scheme %q", u.Scheme)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: u.Scheme == "https", // local printer certificates are commonly self-signed
			ServerName:         u.Hostname(),
		},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 10 * time.Second}
			address := p.Address
			if address == "" {
				address = u.Hostname()
			}
			port := p.Port
			if port == 0 {
				port = defaultPort(u.Scheme, u.Port())
			}
			return d.DialContext(ctx, network, net.JoinHostPort(address, strconv.Itoa(port)))
		},
	}
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URI, bytes.NewReader(payload))
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Content-Type", "application/ipp")
	req.Header.Set("Accept", "application/ipp")
	req.Header.Set("User-Agent", "Gepfrad-Print/0.2")
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("IPP transport error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("printer HTTP status %s", resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	return Decode(raw)
}

func Decode(b []byte) (Response, error) {
	if len(b) < 8 {
		return Response{}, fmt.Errorf("IPP response too short")
	}
	status := binary.BigEndian.Uint16(b[2:4])
	r := Response{Status: status}
	i := 8
	lastName := ""
	for i < len(b) {
		tag := b[i]
		i++
		if tag == TagEnd {
			break
		}
		if tag == TagOperation || tag == TagJob || tag == 0x04 {
			continue
		}
		if i+4 > len(b) {
			return r, fmt.Errorf("truncated attribute")
		}
		nl := int(binary.BigEndian.Uint16(b[i : i+2]))
		i += 2
		if i+nl+2 > len(b) {
			return r, fmt.Errorf("truncated name")
		}
		name := string(b[i : i+nl])
		i += nl
		vl := int(binary.BigEndian.Uint16(b[i : i+2]))
		i += 2
		if i+vl > len(b) {
			return r, fmt.Errorf("truncated value")
		}
		v := b[i : i+vl]
		i += vl
		if name == "" {
			name = lastName
		} else {
			lastName = name
		}
		var x any = string(v)
		switch tag {
		case TagInteger, TagEnum:
			if vl == 4 {
				x = int32(binary.BigEndian.Uint32(v))
			}
		case TagBoolean:
			if vl == 1 {
				x = v[0] != 0
			}
		}
		r.Attributes = append(r.Attributes, Attr{Tag: tag, Name: name, Value: x})
	}
	return r, nil
}

func Find(resp Response, name string) []Attr {
	var out []Attr
	for _, a := range resp.Attributes {
		if strings.EqualFold(a.Name, name) {
			out = append(out, a)
		}
	}
	return out
}

func attrString(resp Response, name string) string {
	attrs := Find(resp, name)
	if len(attrs) == 0 {
		return ""
	}
	return fmt.Sprint(attrs[0].Value)
}

func attrStrings(resp Response, name string) []string {
	attrs := Find(resp, name)
	out := make([]string, 0, len(attrs))
	for _, a := range attrs {
		if s, ok := a.Value.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func FormatStatus(status uint16, resp Response) string {
	message := attrString(resp, "status-message")
	if message != "" {
		return fmt.Sprintf("0x%04x: %s", status, message)
	}
	return fmt.Sprintf("0x%04x", status)
}

func requestID() uint32 {
	return uint32(time.Now().UnixNano())
}

func defaultPort(scheme, explicit string) int {
	if explicit != "" {
		p, _ := strconv.Atoi(explicit)
		return p
	}
	if strings.EqualFold(scheme, "https") {
		return 631
	}
	return 631
}

func hostFromURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func schemeFromURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Scheme
}

func tlsVersion(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "1.3"
	case tls.VersionTLS12:
		return "1.2"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}
