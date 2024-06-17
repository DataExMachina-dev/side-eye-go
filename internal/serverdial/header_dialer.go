package serverdial

import (
	"fmt"
	"net"
)

type headerDialer struct {
	net.Listener
}

// / The header byte for inbound server connections.
// /
// / Note that this choice is arbitrary.
var inboundServerPrefix = string([]byte{1, 1, 1, 9, 1, 1, 1, 0})

func (d *headerDialer) Accept() (net.Conn, error) {
	conn, err := d.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if err := writeHeader(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

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
