package redis

import (
	"context"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
)

func testRedisURL(t *testing.T) string {
	t.Helper()
	rawURL := os.Getenv("HOTKEY_TEST_REDIS_URL")
	if rawURL == "" {
		t.Fatal("HOTKEY_TEST_REDIS_URL is required for Redis integration tests")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse HOTKEY_TEST_REDIS_URL: %v", err)
	}
	if parsed.Scheme != "redis" || parsed.Host == "" {
		t.Fatalf("HOTKEY_TEST_REDIS_URL = %q, want redis URL", rawURL)
	}
	connection, err := net.DialTimeout("tcp", parsed.Host, time.Second)
	if err != nil {
		t.Fatalf("connect real Redis at %s: %v", parsed.Host, err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close Redis probe: %v", err)
	}
	return rawURL
}

func TestVerificationStoreReturnsUnavailableDuringRealDisconnectAndRecoversExistingCode(t *testing.T) {
	realURL := testRedisURL(t)
	parsed, err := url.Parse(realURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newRedisFaultProxy(t, parsed.Host)
	t.Cleanup(proxy.Close)
	parsed.Host = proxy.Address()
	store, err := NewVerificationStoreFromURL(parsed.String(), testVerificationHMACSecret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	email := uniqueVerificationEmail("disconnect-recovery")
	if err := store.CreateCode(ctx, domain.VerificationPurposePasswordReset, email, "314159", time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("CreateCode() before disconnect: %v", err)
	}
	proxy.Stop()
	disconnectedCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := store.ConsumeCode(disconnectedCtx, domain.VerificationPurposePasswordReset, email, "314159"); appErrorCode(err) != sharederrors.CodeUnavailable {
		t.Fatalf("ConsumeCode() during disconnect error = %v, want unavailable", err)
	}
	proxy.Start(t)
	ticket, err := store.ConsumeCode(ctx, domain.VerificationPurposePasswordReset, email, "314159")
	if err != nil || ticket.Email != email || ticket.Purpose != domain.VerificationPurposePasswordReset || ticket.Token == "" {
		t.Fatalf("ConsumeCode() after recovery = %#v/%v", ticket, err)
	}
}

type redisFaultProxy struct {
	target   string
	address  string
	mu       sync.Mutex
	listener net.Listener
	active   map[net.Conn]struct{}
}

func newRedisFaultProxy(t *testing.T, target string) *redisFaultProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start Redis fault proxy: %v", err)
	}
	proxy := &redisFaultProxy{target: target, address: listener.Addr().String(), listener: listener, active: make(map[net.Conn]struct{})}
	go proxy.accept(listener)
	return proxy
}

func (proxy *redisFaultProxy) Address() string { return proxy.address }

func (proxy *redisFaultProxy) Start(t *testing.T) {
	t.Helper()
	proxy.mu.Lock()
	if proxy.listener != nil {
		proxy.mu.Unlock()
		return
	}
	listener, err := net.Listen("tcp", proxy.address)
	if err != nil {
		proxy.mu.Unlock()
		t.Fatalf("restart Redis fault proxy: %v", err)
	}
	proxy.listener = listener
	proxy.mu.Unlock()
	go proxy.accept(listener)
}

func (proxy *redisFaultProxy) Stop() {
	proxy.mu.Lock()
	listener := proxy.listener
	proxy.listener = nil
	connections := make([]net.Conn, 0, len(proxy.active))
	for connection := range proxy.active {
		connections = append(connections, connection)
	}
	proxy.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	for _, connection := range connections {
		_ = connection.Close()
	}
}

func (proxy *redisFaultProxy) Close() { proxy.Stop() }

func (proxy *redisFaultProxy) accept(listener net.Listener) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go proxy.forward(connection)
	}
}

func (proxy *redisFaultProxy) forward(client net.Conn) {
	upstream, err := net.DialTimeout("tcp", proxy.target, time.Second)
	if err != nil {
		_ = client.Close()
		return
	}
	proxy.track(client, true)
	proxy.track(upstream, true)
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
	<-done
	_ = client.Close()
	_ = upstream.Close()
	<-done
	proxy.track(client, false)
	proxy.track(upstream, false)
}

func (proxy *redisFaultProxy) track(connection net.Conn, add bool) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if add {
		proxy.active[connection] = struct{}{}
		return
	}
	delete(proxy.active, connection)
}
