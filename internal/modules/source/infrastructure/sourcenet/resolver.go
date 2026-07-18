// Package sourcenet provides operator-controlled DNS resolution for source
// connectors. Connector-specific dialers remain responsible for SSRF checks
// on every resolved address.
package sourcenet

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

const maxDNSMessageSize = 64 * 1024

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type dohResolver struct {
	endpoint *url.URL
	client   *http.Client
}

func NewResolver(dohEndpoint string) (Resolver, error) {
	if strings.TrimSpace(dohEndpoint) == "" {
		return net.DefaultResolver, nil
	}
	endpoint, err := url.Parse(strings.TrimSpace(dohEndpoint))
	if err != nil || (endpoint.Port() != "" && endpoint.Port() != "443") {
		return nil, errors.New("source DNS-over-HTTPS endpoint must use port 443")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return newDoHResolver(dohEndpoint, &http.Client{Timeout: 10 * time.Second, Transport: transport})
}

func newDoHResolver(rawEndpoint string, client *http.Client) (*dohResolver, error) {
	endpoint, err := url.Parse(strings.TrimSpace(rawEndpoint))
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil {
		return nil, errors.New("source DNS-over-HTTPS endpoint must be an HTTPS URL")
	}
	if client == nil {
		return nil, errors.New("source DNS-over-HTTPS client is required")
	}
	return &dohResolver{endpoint: endpoint, client: client}, nil
}

func (resolver *dohResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	name, err := dnsmessage.NewName(strings.TrimSuffix(strings.TrimSpace(host), ".") + ".")
	if err != nil {
		return nil, errors.New("invalid source DNS name")
	}
	addresses := make([]net.IPAddr, 0, 4)
	var queryErrors []error
	for _, recordType := range []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA} {
		resolved, err := resolver.lookup(ctx, name, recordType)
		if err != nil {
			queryErrors = append(queryErrors, err)
			continue
		}
		addresses = append(addresses, resolved...)
	}
	if len(addresses) == 0 {
		if len(queryErrors) > 0 {
			return nil, errors.Join(queryErrors...)
		}
		return nil, errors.New("source DNS-over-HTTPS response contained no addresses")
	}
	return addresses, nil
}

func (resolver *dohResolver) lookup(ctx context.Context, name dnsmessage.Name, recordType dnsmessage.Type) ([]net.IPAddr, error) {
	queryID, err := randomQueryID()
	if err != nil {
		return nil, errors.New("create source DNS query ID")
	}
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: queryID, RecursionDesired: true})
	if err := builder.StartQuestions(); err != nil {
		return nil, fmt.Errorf("start source DNS question: %w", err)
	}
	if err := builder.Question(dnsmessage.Question{Name: name, Type: recordType, Class: dnsmessage.ClassINET}); err != nil {
		return nil, fmt.Errorf("build source DNS question: %w", err)
	}
	payload, err := builder.Finish()
	if err != nil {
		return nil, fmt.Errorf("finish source DNS question: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, resolver.endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("create source DNS-over-HTTPS request")
	}
	request.Header.Set("Accept", "application/dns-message")
	request.Header.Set("Content-Type", "application/dns-message")
	response, err := resolver.client.Do(request)
	if err != nil {
		return nil, errors.New("perform source DNS-over-HTTPS request")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("source DNS-over-HTTPS returned status %d", response.StatusCode)
	}
	responsePayload, err := io.ReadAll(io.LimitReader(response.Body, maxDNSMessageSize+1))
	if err != nil || len(responsePayload) > maxDNSMessageSize {
		return nil, errors.New("read source DNS-over-HTTPS response")
	}
	return parseAddressResponse(responsePayload, queryID)
}

func parseAddressResponse(payload []byte, queryID uint16) ([]net.IPAddr, error) {
	var parser dnsmessage.Parser
	header, err := parser.Start(payload)
	if err != nil || !header.Response || header.ID != queryID || header.RCode != dnsmessage.RCodeSuccess {
		return nil, errors.New("invalid source DNS-over-HTTPS response")
	}
	if err := parser.SkipAllQuestions(); err != nil {
		return nil, errors.New("invalid source DNS-over-HTTPS questions")
	}
	addresses := make([]net.IPAddr, 0, 4)
	for {
		header, err := parser.AnswerHeader()
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}
		if err != nil {
			return nil, errors.New("invalid source DNS-over-HTTPS answer")
		}
		switch header.Type {
		case dnsmessage.TypeA:
			resource, err := parser.AResource()
			if err != nil {
				return nil, errors.New("invalid source DNS-over-HTTPS A answer")
			}
			addresses = append(addresses, net.IPAddr{IP: net.IP(resource.A[:])})
		case dnsmessage.TypeAAAA:
			resource, err := parser.AAAAResource()
			if err != nil {
				return nil, errors.New("invalid source DNS-over-HTTPS AAAA answer")
			}
			addresses = append(addresses, net.IPAddr{IP: net.IP(resource.AAAA[:])})
		default:
			if err := parser.SkipAnswer(); err != nil {
				return nil, errors.New("invalid source DNS-over-HTTPS answer body")
			}
		}
	}
	return addresses, nil
}

func randomQueryID() (uint16, error) {
	var value [2]byte
	if _, err := rand.Read(value[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(value[:]), nil
}
