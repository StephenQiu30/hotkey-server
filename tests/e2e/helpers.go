package e2e_test

import (
	"context"
	"fmt"
	"net"
	"net/url"
)

// pingTCP checks if a TCP port is accepting connections.
func pingTCP(ctx context.Context, addr string) error {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp dial %s: %w", addr, err)
	}
	defer conn.Close()
	return nil
}

// redisPing performs a PING command over a raw TCP connection to Redis.
func redisPing(ctx context.Context, rawURL string) error {
	// Handle bare host:port input (e.g. "127.0.0.1:16379") which url.Parse
	// puts into Host only when a scheme is present.
	addr := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		addr = u.Host
	}

	if addr == "" {
		addr = "127.0.0.1:6379"
	} else if _, _, err := net.SplitHostPort(addr); err != nil {
		// Host is just a hostname without port — append default Redis port.
		addr = net.JoinHostPort(addr, "6379")
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial redis %s: %w", addr, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := conn.Write([]byte("PING\r\n")); err != nil {
		return fmt.Errorf("redis PING write: %w", err)
	}
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("redis PING read: %w", err)
	}
	resp := string(buf[:n])
	if resp != "+PONG\r\n" && resp != "+PONG" {
		return fmt.Errorf("redis PING unexpected response: %q", resp)
	}
	return nil
}
