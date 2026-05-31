package redis

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	DialTimeout time.Duration
}

type Client struct {
	addr        string
	dialTimeout time.Duration
}

var dialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

func NewClient(rawURL string, opts Options) *Client {
	addr := rawURL
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Host != "" {
		addr = parsed.Host
		if _, _, splitErr := net.SplitHostPort(addr); splitErr != nil {
			addr = net.JoinHostPort(addr, "6379")
		}
	}
	timeout := opts.DialTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{addr: addr, dialTimeout: timeout}
}

func (c *Client) Ping(ctx context.Context) error {
	value, err := c.do(ctx, "PING")
	if err != nil {
		return err
	}
	if string(value) != "PONG" {
		return errors.New("unexpected redis ping response")
	}
	return nil
}

func (c *Client) SetNX(ctx context.Context, key string, value []byte) (bool, error) {
	reply, err := c.do(ctx, "SET", key, string(value), "NX")
	if err != nil {
		return false, err
	}
	return string(reply) == "OK", nil
}

func (c *Client) Set(ctx context.Context, key string, value []byte) error {
	_, err := c.do(ctx, "SET", key, string(value))
	return err
}

func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	return c.do(ctx, "GET", key)
}

func (c *Client) Del(ctx context.Context, key string) error {
	_, err := c.do(ctx, "DEL", key)
	return err
}

func (c *Client) LPush(ctx context.Context, key string, value []byte) error {
	_, err := c.do(ctx, "LPUSH", key, string(value))
	return err
}

func (c *Client) RPop(ctx context.Context, key string) ([]byte, error) {
	return c.do(ctx, "RPOP", key)
}

func (c *Client) do(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()

	conn, err := dialContext(ctx, "tcp", c.addr)
	if err != nil {
		return nil, err
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, errors.Join(err, conn.Close())
		}
	}

	if _, err := conn.Write(encodeCommand(args...)); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	reply, err := readReply(bufio.NewReader(conn))
	return reply, errors.Join(err, conn.Close())
}

func encodeCommand(args ...string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&buf, "$%d\r\n%s\r\n", len(arg), arg)
	}
	return buf.Bytes()
}

func readReply(reader *bufio.Reader) ([]byte, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		line, err := reader.ReadString('\n')
		return []byte(strings.TrimSuffix(line, "\r\n")), err
	case ':':
		line, err := reader.ReadString('\n')
		return []byte(strings.TrimSuffix(line, "\r\n")), err
	case '$':
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSuffix(line, "\r\n"))
		if err != nil {
			return nil, err
		}
		if size < 0 {
			return nil, errors.New("redis nil reply")
		}
		body := make([]byte, size+2)
		if _, err := io.ReadFull(reader, body); err != nil {
			return nil, err
		}
		return body[:size], nil
	case '-':
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		return nil, errors.New(strings.TrimSuffix(line, "\r\n"))
	default:
		return nil, fmt.Errorf("unsupported redis reply prefix %q", prefix)
	}
}
