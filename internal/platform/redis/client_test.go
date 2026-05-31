package redis

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestClientUnavailableReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := NewClient("redis://127.0.0.1:1/0", Options{DialTimeout: 10 * time.Millisecond})
	if err := client.Ping(ctx); err == nil {
		t.Fatal("expected unavailable redis to return an error")
	}
}

func TestClientReturnsCloseErrorAfterSuccessfulReply(t *testing.T) {
	closeErr := errors.New("close failed")
	previousDialContext := dialContext
	dialContext = func(context.Context, string, string) (net.Conn, error) {
		return &closeErrorConn{
			Buffer:   bytes.NewBufferString("+PONG\r\n"),
			closeErr: closeErr,
		}, nil
	}
	t.Cleanup(func() {
		dialContext = previousDialContext
	})

	client := NewClient("redis://127.0.0.1:6379/0", Options{DialTimeout: time.Second})

	err := client.Ping(context.Background())
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected close error, got %v", err)
	}
}

type closeErrorConn struct {
	*bytes.Buffer
	closeErr error
}

func (c *closeErrorConn) Read(b []byte) (int, error) {
	return c.Buffer.Read(b)
}

func (c *closeErrorConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func (c *closeErrorConn) Close() error {
	return c.closeErr
}

func (c *closeErrorConn) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *closeErrorConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *closeErrorConn) SetDeadline(time.Time) error {
	return nil
}

func (c *closeErrorConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *closeErrorConn) SetWriteDeadline(time.Time) error {
	return nil
}
