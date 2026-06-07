package e2e_test

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// smtpSink implements SMTPSink as an in-memory email capture server.
type smtpSink struct {
	mu      sync.Mutex
	addr    string
	records []EmailRecord
}

func newSMTPSinkImpl() (*smtpSink, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("smtp sink listen: %w", err)
	}
	s := &smtpSink{
		addr:    ln.Addr().String(),
		records: make([]EmailRecord, 0),
	}
	// Accept connections in background to keep port open
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Minimal SMTP handshake: 220 -> EHLO -> MAIL -> RCPT -> DATA -> QUIT
			handleSMTPConn(conn, s)
		}
	}()
	return s, nil
}

func handleSMTPConn(conn net.Conn, sink *smtpSink) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send greeting
	fmt.Fprintf(conn, "220 e2e-smtp-sink ready\r\n")

	buf := make([]byte, 4096)
	var from, to, subject, body string
	inData := false

	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		line := string(buf[:n])

		if inData {
			if line == ".\r\n" || line == ".\n" || line == "." {
				inData = false
				subject = extractSubject(body) // Extract subject from body headers
				sink.mu.Lock()
				sink.records = append(sink.records, EmailRecord{
					From:    from,
					To:      to,
					Subject: subject,
					Body:    body,
					SentAt:  time.Now(),
				})
				sink.mu.Unlock()
				fmt.Fprintf(conn, "250 OK\r\n")
				body = ""
				subject = ""
			} else {
				body += line
			}
			continue
		}

		switch {
		case len(line) >= 4 && line[:4] == "EHLO":
			fmt.Fprintf(conn, "250-e2e-smtp-sink\r\n250 OK\r\n")
		case len(line) >= 4 && line[:4] == "HELO":
			fmt.Fprintf(conn, "250 OK\r\n")
		case len(line) >= 9 && line[:9] == "MAIL FROM":
			from = extractAngle(line)
			fmt.Fprintf(conn, "250 OK\r\n")
		case len(line) >= 7 && line[:7] == "RCPT TO":
			to = extractAngle(line)
			fmt.Fprintf(conn, "250 OK\r\n")
		case len(line) >= 4 && line[:4] == "DATA":
			fmt.Fprintf(conn, "354 End data with <CR><LF>.<CR><LF>\r\n")
			inData = true
		case len(line) >= 4 && line[:4] == "QUIT":
			fmt.Fprintf(conn, "221 Bye\r\n")
			return
		default:
			fmt.Fprintf(conn, "250 OK\r\n")
		}
	}
}

// extractSubject extracts the Subject header from message body.
func extractSubject(body string) string {
	lines := strings.Split(body, "\r\n")
	for _, l := range lines {
		if len(l) > 8 && strings.HasPrefix(strings.ToLower(l), "subject:") {
			return strings.TrimSpace(l[8:])
		}
	}
	// Try \n if \r\n failed
	lines = strings.Split(body, "\n")
	for _, l := range lines {
		if len(l) > 8 && strings.HasPrefix(strings.ToLower(l), "subject:") {
			return strings.TrimSpace(l[8:])
		}
	}
	return ""
}

// extractAngle extracts the address from <addr> format.
func extractAngle(line string) string {
	start := -1
	end := -1
	for i, c := range line {
		if c == '<' {
			start = i + 1
		} else if c == '>' && start >= 0 {
			end = i
			break
		}
	}
	if start >= 0 && end > start {
		return line[start:end]
	}
	return ""
}

func (s *smtpSink) Addr() string {
	return s.addr
}

func (s *smtpSink) Records() []EmailRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]EmailRecord, len(s.records))
	copy(cp, s.records)
	return cp
}

func (s *smtpSink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make([]EmailRecord, 0)
}
