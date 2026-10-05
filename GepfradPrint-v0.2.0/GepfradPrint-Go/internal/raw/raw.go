package raw

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

func Send(ctx context.Context, host string, port int, data []byte) error {
	if port == 0 {
		port = 9100
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	c, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}
	defer c.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(deadline)
	} else {
		_ = c.SetWriteDeadline(time.Now().Add(60 * time.Second))
	}
	if _, err = io.Copy(c, io.LimitReader(bytesReader(data), int64(len(data)))); err != nil {
		return err
	}
	return nil
}

type byteReader struct {
	b []byte
	i int
}

func bytesReader(b []byte) io.Reader { return &byteReader{b: b} }
func (r *byteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
