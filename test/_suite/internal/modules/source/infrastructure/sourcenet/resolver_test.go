package sourcenet

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestDoHResolverReturnsPublicAddresses(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("ReadAll(): %v", err)
		}
		var parser dnsmessage.Parser
		header, err := parser.Start(payload)
		if err != nil {
			t.Fatalf("Start(): %v", err)
		}
		question, err := parser.Question()
		if err != nil {
			t.Fatalf("Question(): %v", err)
		}
		builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: header.ID, Response: true, RecursionAvailable: true})
		builder.EnableCompression()
		if err := builder.StartQuestions(); err != nil {
			t.Fatalf("StartQuestions(): %v", err)
		}
		if err := builder.Question(question); err != nil {
			t.Fatalf("Question(): %v", err)
		}
		if err := builder.StartAnswers(); err != nil {
			t.Fatalf("StartAnswers(): %v", err)
		}
		switch question.Type {
		case dnsmessage.TypeA:
			err = builder.AResource(
				dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
				dnsmessage.AResource{A: [4]byte{151, 101, 3, 42}},
			)
		case dnsmessage.TypeAAAA:
			err = builder.AAAAResource(
				dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeAAAA, Class: dnsmessage.ClassINET, TTL: 60},
				dnsmessage.AAAAResource{AAAA: [16]byte{0x26, 0x06, 0x47, 0x00, 0x47, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0x11, 0x11}},
			)
		}
		if err != nil {
			t.Fatalf("resource: %v", err)
		}
		response, err := builder.Finish()
		if err != nil {
			t.Fatalf("Finish(): %v", err)
		}
		writer.Header().Set("Content-Type", "application/dns-message")
		_, _ = writer.Write(response)
	}))
	defer server.Close()

	resolver, err := newDoHResolver(server.URL, server.Client())
	if err != nil {
		t.Fatalf("newDoHResolver(): %v", err)
	}
	addresses, err := resolver.LookupIPAddr(context.Background(), "export.arxiv.org")
	if err != nil {
		t.Fatalf("LookupIPAddr(): %v", err)
	}
	if len(addresses) != 2 || !addresses[0].IP.Equal(net.ParseIP("151.101.3.42")) || !addresses[1].IP.Equal(net.ParseIP("2606:4700:4700::1111")) {
		t.Fatalf("LookupIPAddr() = %#v, want A and AAAA answers", addresses)
	}
}

func TestDoHResolverRejectsInsecureEndpoint(t *testing.T) {
	t.Parallel()

	if _, err := newDoHResolver("http://resolver.example/dns-query", http.DefaultClient); err == nil {
		t.Fatal("newDoHResolver() error = nil, want insecure endpoint rejection")
	}
}
