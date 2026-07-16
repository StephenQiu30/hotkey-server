package minio_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionminio "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/minio"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
)

func TestStorePutTextReusesVerifiedObject(t *testing.T) {
	t.Parallel()

	fake := newS3Fake(t)
	store := newStore(t, fake.server.URL)
	object := evidenceObject(t, 41, "the preserved source text")

	first, err := store.PutText(context.Background(), object)
	if err != nil {
		t.Fatalf("PutText(first) error = %v", err)
	}
	second, err := store.PutText(context.Background(), object)
	if err != nil {
		t.Fatalf("PutText(retry) error = %v", err)
	}
	if first != second {
		t.Fatalf("PutText receipts = %#v and %#v, want deterministic reuse", first, second)
	}
	if fake.putCount() != 1 {
		t.Fatalf("PUT count = %d, want exactly one upload", fake.putCount())
	}
	if first.ObjectKey != ingestionminio.EvidenceObjectKey(object.SourceConnectionID, object.SHA256) || first.SHA256 != object.SHA256 || first.SizeBytes != int64(len(object.Text)) {
		t.Fatalf("PutText receipt = %#v, want verified deterministic object", first)
	}
}

func TestStorePutTextRejectsSHAMismatch(t *testing.T) {
	t.Parallel()

	fake := newS3Fake(t)
	store := newStore(t, fake.server.URL)
	object := evidenceObject(t, 42, "text that must not be relabeled")
	object.SHA256 = strings.Repeat("0", 64)

	if _, err := store.PutText(context.Background(), object); err == nil {
		t.Fatal("PutText() error = nil, want SHA mismatch rejection")
	}
	if fake.putCount() != 0 {
		t.Fatalf("PUT count = %d, want no write after SHA mismatch", fake.putCount())
	}
}

func TestStorePutTextHonorsContextTimeout(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodHead {
			close(started)
			<-request.Context().Done()
			return
		}
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	store := newStore(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := store.PutText(ctx, evidenceObject(t, 43, "timed out source text"))
	if err == nil {
		t.Fatal("PutText() error = nil, want context deadline")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("PutText() never attempted object verification")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("PutText() error = %v, want context deadline exceeded", err)
	}
}

func TestStoreDeletePropagatesFailure(t *testing.T) {
	t.Parallel()

	fake := newS3Fake(t)
	fake.deleteStatus = http.StatusInternalServerError
	store := newStore(t, fake.server.URL)
	key := ingestionminio.EvidenceObjectKey(44, strings.Repeat("a", 64))

	if err := store.Delete(context.Background(), key); err == nil {
		t.Fatal("Delete() error = nil, want storage failure")
	}
}

func TestStoreRejectsEmptyText(t *testing.T) {
	t.Parallel()

	fake := newS3Fake(t)
	store := newStore(t, fake.server.URL)
	sha := sha256.Sum256(nil)
	object := ingestiondomain.EvidenceObject{
		SourceConnectionID: 45,
		SHA256:             fmt.Sprintf("%x", sha),
		ObjectKey:          ingestionminio.EvidenceObjectKey(45, fmt.Sprintf("%x", sha)),
	}

	if _, err := store.PutText(context.Background(), object); err == nil {
		t.Fatal("PutText() error = nil, want empty body rejection")
	}
	if fake.putCount() != 0 {
		t.Fatalf("PUT count = %d, want no empty-object upload", fake.putCount())
	}
}

func TestEvidenceObjectKeyUsesSourceScopedSHAPath(t *testing.T) {
	t.Parallel()

	sha := strings.Repeat("a", 64)
	if got, want := ingestionminio.EvidenceObjectKey(46, sha), "evidence/v1/46/aa/"+sha+".txt"; got != want {
		t.Fatalf("EvidenceObjectKey() = %q, want %q", got, want)
	}
	if got := ingestionminio.EvidenceObjectKey(0, sha); got != "" {
		t.Fatalf("EvidenceObjectKey(invalid source) = %q, want empty", got)
	}
	if got := ingestionminio.EvidenceObjectKey(46, "invalid"); got != "" {
		t.Fatalf("EvidenceObjectKey(invalid SHA) = %q, want empty", got)
	}
}

func evidenceObject(t *testing.T, sourceID int64, text string) ingestiondomain.EvidenceObject {
	t.Helper()
	sha := sha256.Sum256([]byte(text))
	digest := fmt.Sprintf("%x", sha)
	return ingestiondomain.EvidenceObject{
		SourceConnectionID: sourceID,
		ObjectKey:          ingestionminio.EvidenceObjectKey(sourceID, digest),
		Text:               text,
		SHA256:             digest,
	}
}

func newStore(t *testing.T, rawURL string) *ingestionminio.Store {
	t.Helper()
	endpoint, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse fake endpoint: %v", err)
	}
	store, err := ingestionminio.NewStore(config.MinIOConfig{
		Endpoint:  endpoint.Host,
		AccessKey: "test-access-key",
		SecretKey: "test-secret-key",
		Bucket:    "evidence-test",
		UseSSL:    false,
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	return store
}

type s3Fake struct {
	server       *httptest.Server
	mu           sync.Mutex
	objects      map[string]fakeObject
	puts         int
	deleteStatus int
}

type fakeObject struct {
	text string
	sha  string
}

func newS3Fake(t *testing.T) *s3Fake {
	t.Helper()
	fake := &s3Fake{objects: make(map[string]fakeObject)}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	t.Cleanup(fake.server.Close)
	return fake
}

func (fake *s3Fake) putCount() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.puts
}

func (fake *s3Fake) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	key := strings.TrimPrefix(request.URL.EscapedPath(), "/evidence-test/")
	if key == request.URL.EscapedPath()[1:] {
		writeS3Error(writer, http.StatusNotFound, "NoSuchBucket")
		return
	}
	key, _ = url.PathUnescape(key)

	switch request.Method {
	case http.MethodHead:
		fake.mu.Lock()
		object, ok := fake.objects[key]
		fake.mu.Unlock()
		if !ok {
			writeS3Error(writer, http.StatusNotFound, "NoSuchKey")
			return
		}
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(object.text)))
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.Header().Set("X-Amz-Meta-Sha256", object.sha)
		writer.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		writer.WriteHeader(http.StatusOK)
	case http.MethodPut:
		text, err := readS3Payload(request)
		if err != nil {
			writeS3Error(writer, http.StatusBadRequest, "InvalidRequest")
			return
		}
		fake.mu.Lock()
		fake.puts++
		fake.objects[key] = fakeObject{text: string(text), sha: request.Header.Get("X-Amz-Meta-Sha256")}
		fake.mu.Unlock()
		writer.Header().Set("ETag", "\"fixture-etag\"")
		writer.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if fake.deleteStatus != 0 && fake.deleteStatus != http.StatusNoContent {
			writeS3Error(writer, fake.deleteStatus, "InternalError")
			return
		}
		fake.mu.Lock()
		delete(fake.objects, key)
		fake.mu.Unlock()
		writer.WriteHeader(http.StatusNoContent)
	default:
		writeS3Error(writer, http.StatusMethodNotAllowed, "MethodNotAllowed")
	}
}

func readS3Payload(request *http.Request) ([]byte, error) {
	if !strings.HasPrefix(request.Header.Get("X-Amz-Content-Sha256"), "STREAMING-AWS4-HMAC-SHA256-PAYLOAD") {
		return io.ReadAll(request.Body)
	}

	reader := bufio.NewReader(request.Body)
	payload := make([]byte, 0, request.ContentLength)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		length, err := strconv.ParseInt(strings.Split(strings.TrimSuffix(line, "\r\n"), ";")[0], 16, 64)
		if err != nil || length < 0 {
			return nil, fmt.Errorf("invalid streaming chunk length")
		}
		if length == 0 {
			_, err = reader.ReadString('\n')
			return payload, err
		}
		chunk := make([]byte, length)
		if _, err := io.ReadFull(reader, chunk); err != nil {
			return nil, err
		}
		if ending, err := reader.ReadString('\n'); err != nil || ending != "\r\n" {
			return nil, fmt.Errorf("invalid streaming chunk terminator")
		}
		payload = append(payload, chunk...)
	}
}

func writeS3Error(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/xml")
	writer.WriteHeader(status)
	_, _ = fmt.Fprintf(writer, "<Error><Code>%s</Code><Message>fixture</Message></Error>", code)
}
