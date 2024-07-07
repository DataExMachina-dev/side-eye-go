package serverdial

import (
	"context"
	"fmt"
	"net"
)

type headerDialer struct {
	d dialer
}

var _ dialer = &headerDialer{}

func (d *headerDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	if err := writeHeader(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// / The header byte for inbound server connections.
// /
// / Note that this choice is arbitrary.
var inboundServerPrefix = string([]byte{1, 1, 1, 9, 1, 1, 1, 0})

func writeHeader(conn net.Conn) error {
	toWrite := []byte(inboundServerPrefix)
	for len(toWrite) > 0 {
		n, err := conn.Write(toWrite)
		if err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
		toWrite = toWrite[n:]
	}
	return nil
}
