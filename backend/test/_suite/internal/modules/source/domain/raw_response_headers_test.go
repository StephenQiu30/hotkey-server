package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestEvidenceSnapshotKeepsOnlyAllowlistedResponseHeaders(t *testing.T) {
	t.Parallel()

	profile, err := NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if err != nil {
		t.Fatal(err)
	}
	rawHeaders := map[string][]string{
		"content-type": {"application/rss+xml"}, "etag": {`"v1"`},
		"last-modified": {"Sun, 09 Aug 2026 05:00:00 GMT"}, "date": {"Sun, 09 Aug 2026 06:00:00 GMT"},
		"link": {"<https://feed.example/page/2>; rel=next"}, "retry-after": {"120"},
		"authorization": {"Bearer secret"}, "cookie": {"session=secret"},
		"set-cookie": {"session=secret"}, "x-provider-payload": {"secret"},
	}
	headers, err := NewRawResponseHeaders(rawHeaders)
	if err != nil {
		t.Fatal(err)
	}
	input := EvidenceSnapshot{
		Payload: []byte("response"), MIMEType: "application/rss+xml", StatusCode: 200,
		RequestedURL: "https://feed.example/source.xml", FinalURL: "https://feed.example/source.xml",
		CapturedAt:              time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC),
		CollectorProfileVersion: profile,
		ResponseHeaders:         headers,
	}
	snapshot, err := NewEvidenceSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"Content-Type": {"application/rss+xml"}, "ETag": {`"v1"`},
		"Last-Modified": {"Sun, 09 Aug 2026 05:00:00 GMT"}, "Date": {"Sun, 09 Aug 2026 06:00:00 GMT"},
		"Link": {"<https://feed.example/page/2>; rel=next"}, "Retry-After": {"120"},
	}
	if got := snapshot.ResponseHeaders.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("response headers = %#v, want %#v", got, want)
	}

	rawHeaders["etag"][0] = `"mutated"`
	if got := snapshot.ResponseHeaders.Values()["ETag"][0]; got != `"v1"` {
		t.Fatalf("snapshot header mutated through input map: %q", got)
	}
}

func TestEvidenceSnapshotRejectsUnsafeAllowlistedHeaderValues(t *testing.T) {
	t.Parallel()

	_, err := NewRawResponseHeaders(map[string][]string{"ETag": {"safe\r\nSet-Cookie: secret"}})
	if err == nil {
		t.Fatal("NewRawResponseHeaders() accepted a response-splitting header")
	}
}
