// Package smtpfixture provides an isolated TLS SMTP server for credential and
// delivery acceptance tests. It never records message bodies or credentials.
package smtpfixture

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type Server struct {
	listener   net.Listener
	tlsConfig  *tls.Config
	mu         sync.RWMutex
	passwords  map[string]string
	deliveries int
	done       chan struct{}
}

func New(t testing.TB) *Server {
	t.Helper()
	certificate, roots := certificateFixture(t)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("start isolated SMTP fixture: %v", err)
	}
	server := &Server{
		listener:  listener,
		tlsConfig: &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12},
		passwords: make(map[string]string),
		done:      make(chan struct{}),
	}
	go server.serve()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-server.done:
		case <-time.After(3 * time.Second):
			t.Error("isolated SMTP fixture did not stop")
		}
	})
	return server
}

func (server *Server) Endpoint() (string, int) {
	host, portValue, _ := net.SplitHostPort(server.listener.Addr().String())
	port, _ := strconv.Atoi(portValue)
	return host, port
}

func (server *Server) TLSConfig() *tls.Config { return server.tlsConfig.Clone() }

func (server *Server) SetCredentials(credentials map[string]string) {
	server.mu.Lock()
	defer server.mu.Unlock()
	server.passwords = make(map[string]string, len(credentials))
	for username, password := range credentials {
		server.passwords[username] = password
	}
}

func (server *Server) Deliveries() int {
	server.mu.RLock()
	defer server.mu.RUnlock()
	return server.deliveries
}

func (server *Server) serve() {
	defer close(server.done)
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}
		go server.handle(connection)
	}
}

func (server *Server) handle(connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	writeSMTPLine(writer, "220 localhost ESMTP")
	authenticated := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO ") || strings.HasPrefix(upper, "HELO "):
			_, _ = writer.WriteString("250-localhost\r\n250-AUTH PLAIN\r\n250 8BITMIME\r\n")
			_ = writer.Flush()
		case strings.HasPrefix(upper, "AUTH PLAIN "):
			authenticated = server.authorize(strings.TrimSpace(line[len("AUTH PLAIN "):]))
			if authenticated {
				writeSMTPLine(writer, "235 2.7.0 authenticated")
			} else {
				writeSMTPLine(writer, "535 5.7.8 authentication rejected")
			}
		case strings.HasPrefix(upper, "MAIL FROM:"):
			if authenticated {
				writeSMTPLine(writer, "250 2.1.0 sender accepted")
			} else {
				writeSMTPLine(writer, "530 5.7.0 authentication required")
			}
		case strings.HasPrefix(upper, "RCPT TO:"):
			if authenticated {
				writeSMTPLine(writer, "250 2.1.5 recipient accepted")
			} else {
				writeSMTPLine(writer, "530 5.7.0 authentication required")
			}
		case upper == "DATA":
			if !authenticated {
				writeSMTPLine(writer, "530 5.7.0 authentication required")
				continue
			}
			writeSMTPLine(writer, "354 end with <CRLF>.<CRLF>")
			for {
				bodyLine, readErr := reader.ReadString('\n')
				if readErr != nil {
					return
				}
				if strings.TrimRight(bodyLine, "\r\n") == "." {
					break
				}
			}
			server.mu.Lock()
			server.deliveries++
			server.mu.Unlock()
			writeSMTPLine(writer, "250 2.0.0 accepted")
		case upper == "QUIT":
			writeSMTPLine(writer, "221 2.0.0 bye")
			return
		default:
			writeSMTPLine(writer, "502 5.5.2 unsupported")
		}
	}
}

func (server *Server) authorize(encoded string) bool {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	parts := strings.Split(string(payload), "\x00")
	if len(parts) != 3 {
		return false
	}
	server.mu.RLock()
	defer server.mu.RUnlock()
	password, found := server.passwords[parts[1]]
	return found && password == parts[2]
}

func writeSMTPLine(writer *bufio.Writer, line string) {
	_, _ = writer.WriteString(line + "\r\n")
	_ = writer.Flush()
}

func certificateFixture(t testing.TB) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate isolated SMTP private key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create isolated SMTP certificate: %v", err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	certificate, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil {
		t.Fatalf("load isolated SMTP certificate: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certificatePEM)
	return certificate, roots
}
