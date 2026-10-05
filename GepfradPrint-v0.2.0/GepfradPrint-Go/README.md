# Gepfrad Print

A user-space printer client written in Go. It deliberately does **not** use the Windows printing subsystem, Windows printer APIs, .NET, or a system-installed SDK.

## Current build

Version 0.2 adds the pieces needed for printers such as the Epson ET-2720 that advertise `_ipps._tcp.local`:

- mDNS/DNS-SD discovery for `_ipp._tcp`, `_ipps._tcp`, and `_printer._tcp`
- PTR/SRV/TXT/A record parsing, including the IPP `rp` path
- IPPS (IPP over TLS) on TCP 631
- Hand-written IPP binary encoding/decoding
- `Get-Printer-Attributes` capability probing
- More correct IPP print attributes (orientation enum, color keyword, duplex `sides` keyword)
- Printer diagnostics in the web UI for DNS/address, TCP, TLS, and IPP
- Real transport/protocol errors shown on failed jobs instead of only `Failed 0%`
- Raw TCP/9100 transport
- User-space print queue and cancellation
- PDF passthrough and the existing TXT/HTML text renderer
- Persistent user-owned JSON state

IPPS certificates are encrypted but not chain-verified in this local-printer build because consumer printers commonly use locally issued or self-signed certificates. The diagnostics page calls this out explicitly.

LPR service discovery is recognized, but LPR job transmission is not yet implemented.

## Run without administrator privileges

The distributed Windows executable is already built; the user does **not** need Go installed. Run `GepfradPrint.exe` and open `http://127.0.0.1:17842/`.

For developers with a portable Go toolchain:

```powershell
go test ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X main.version=0.2.0' -o GepfradPrint.exe ./cmd/gepfradprint
```

The application binds only to loopback for its UI and does not request elevation.
