package ipp

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	b := []byte{2, 0, 0, 0, 0, 0, 0, 1, 3}
	r, e := Decode(b)
	if e != nil || r.Status != 0 {
		t.Fatalf("decode: %#v %v", r, e)
	}
}

func TestEncodingHeader(t *testing.T) {
	b := header(OpPrintJob, 7).Bytes()
	if binary.BigEndian.Uint16(b[2:4]) != OpPrintJob || binary.BigEndian.Uint32(b[4:8]) != 7 {
		t.Fatal("bad header")
	}
}

func TestDecodeRepeatedValue(t *testing.T) {
	b := []byte{2, 0, 0, 0, 0, 0, 0, 1, TagOperation}
	encStringRaw := func(tag byte, name, value string) {
		b = append(b, tag, 0, byte(len(name)/256), byte(len(name)%256))
		b = append(b, []byte(name)...)
		b = append(b, 0, byte(len(value)))
		b = append(b, []byte(value)...)
	}
	// Build directly to avoid depending on a helper that mutates a buffer type.
	_ = encStringRaw
	b = append(b, TagKeyword, 0, 3, 'f', 'o', 'o', 0, 1, 'a')
	b = append(b, TagKeyword, 0, 0, 0, 1, 'b')
	b = append(b, TagEnd)
	r, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	vals := Find(r, "foo")
	if len(vals) != 2 || vals[0].Value != "a" || vals[1].Value != "b" {
		t.Fatalf("repeated values = %#v", vals)
	}
}

func TestIPPSGetPrinterAttributes(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/ipp" {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		payload := []byte{2, 0, 0, 0, 0, 0, 0, 1}
		payload = append(payload, TagEnd)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	portText := u.Port()
	port, _ := strconv.Atoi(portText)
	resp, err := GetPrinterAttributesEndpoint(context.Background(), Endpoint{
		URI:     srv.URL + "/ipp/print",
		Host:    u.Hostname(),
		Address: u.Hostname(),
		Port:    port,
	})
	if err != nil {
		t.Fatalf("IPPS request failed: %v", err)
	}
	if resp.Status != 0 {
		t.Fatalf("status = %x", resp.Status)
	}
}

func TestPrintColorAndSidesAreKeywords(t *testing.T) {
	b := header(OpPrintJob, 7)
	b.WriteByte(TagOperation)
	encString(b, TagKeyword, "print-color-mode", "color")
	encString(b, TagKeyword, "sides", "two-sided-long-edge")
	b.WriteByte(TagEnd)
	got := string(b.Bytes())
	for _, want := range []string{"print-color-mode", "two-sided-long-edge"} {
		if !strings.Contains(got, want) {
			t.Fatalf("request missing %q", want)
		}
	}
}
