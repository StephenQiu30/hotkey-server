package e2e_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// smtpSink implements SMTPSink as an in-memory email capture server.
type smtpSink struct {
	mu       sync.Mutex
	addr     string
	listener net.Listener
	records  []EmailRecord
}

// newSMTPSinkImpl creates and starts a new instance of smtpSink on a random local port.
func newSMTPSinkImpl() (*smtpSink, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("smtp sink listen: %w", err)
	}
	s := &smtpSink{
		addr:     ln.Addr().String(),
		listener: ln,
		records:  make([]EmailRecord, 0),
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

// handleSMTPConn manages a single SMTP connection, following a minimal protocol flow to capture emails.
func handleSMTPConn(conn net.Conn, sink *smtpSink) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send greeting
	fmt.Fprintf(conn, "220 e2e-smtp-sink ready\r\n")

	scanner := bufio.NewScanner(conn)
	var from, to, subject, body string
	inData := false

	for scanner.Scan() {
		line := scanner.Text()

		if inData {
			if line == "." {
				inData = false
				subject = extractSubject(body)
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
			} else {
				body += line + "\r\n"
			}
			continue
		}

		upperLine := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upperLine, "EHLO"):
			fmt.Fprintf(conn, "250-e2e-smtp-sink\r\n250 OK\r\n")
		case strings.HasPrefix(upperLine, "HELO"):
			fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upperLine, "MAIL FROM"):
			from = extractAngle(line)
			fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upperLine, "RCPT TO"):
			to = extractAngle(line)
			fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upperLine, "DATA"):
			fmt.Fprintf(conn, "354 End data with <CR><LF>.<CR><LF>\r\n")
			inData = true
		case strings.HasPrefix(upperLine, "QUIT"):
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

// Addr returns the network address the SMTP sink is listening on.
func (s *smtpSink) Addr() string {
	return s.addr
}

// Records returns a copy of all captured email records.
func (s *smtpSink) Records() []EmailRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]EmailRecord, len(s.records))
	copy(cp, s.records)
	return cp
}

// Reset clears all captured email records from the sink.
func (s *smtpSink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make([]EmailRecord, 0)
}

// Close stops the SMTP sink listener and releases resources.
func (s *smtpSink) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
