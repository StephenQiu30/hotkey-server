package minio

import (
	"strings"
	"testing"
	"time"

	miniosdk "github.com/minio/minio-go/v7"

	"github.com/StephenQiu30/hotkey-server/internal/domain/objectstorage"
)

func TestNewClient_EmptyEndpoint(t *testing.T) {
	_, err := NewClient(Config{
		Endpoint:  "",
		AccessKey: "key",
		SecretKey: "secret",
		Bucket:    "bucket",
	})
	if err == nil {
		t.Error("NewClient with empty endpoint should fail")
	}
}

func TestNewClient_EmptyBucket(t *testing.T) {
	_, err := NewClient(Config{
		Endpoint:  "localhost:9000",
		AccessKey: "key",
		SecretKey: "secret",
		Bucket:    "",
	})
	if err == nil {
		t.Error("NewClient with empty bucket should fail")
	}
}

func TestNewClient_EmptyAccessKey(t *testing.T) {
	_, err := NewClient(Config{
		Endpoint:  "localhost:9000",
		AccessKey: "",
		SecretKey: "secret",
		Bucket:    "bucket",
	})
	if err == nil {
		t.Error("NewClient with empty access key should fail")
	}
}

func TestNewClient_EmptySecretKey(t *testing.T) {
	_, err := NewClient(Config{
		Endpoint:  "localhost:9000",
		AccessKey: "key",
		SecretKey: "",
		Bucket:    "bucket",
	})
	if err == nil {
		t.Error("NewClient with empty secret key should fail")
	}
}

func TestNewClient_ValidConfig(t *testing.T) {
	client, err := NewClient(Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "test-bucket",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned nil client")
	}
}

func TestNewClient_SetsBucket(t *testing.T) {
	client, err := NewClient(Config{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "my-bucket",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.bucket != "my-bucket" {
		t.Errorf("client.bucket = %q, want %q", client.bucket, "my-bucket")
	}
}

// --- SnapshotContent ---

func TestSnapshotContent_ContainsTitle(t *testing.T) {
	data := SnapshotContent("Breaking News", "Some content", "https://example.com/1")
	if !strings.Contains(string(data), "Breaking News") {
		t.Error("SnapshotContent should contain title")
	}
}

func TestSnapshotContent_ContainsURL(t *testing.T) {
	url := "https://example.com/post/42"
	data := SnapshotContent("Title", "snippet", url)
	if !strings.Contains(string(data), url) {
		t.Error("SnapshotContent should contain original URL")
	}
}

func TestSnapshotContent_ContainsSnippet(t *testing.T) {
	snippet := "This is the article body text."
	data := SnapshotContent("Title", snippet, "https://example.com")
	if !strings.Contains(string(data), snippet) {
		t.Error("SnapshotContent should contain snippet")
	}
}

func TestSnapshotContent_EmptyInputs(t *testing.T) {
	data := SnapshotContent("", "", "")
	if len(data) == 0 {
		t.Error("SnapshotContent with empty inputs should still produce output")
	}
}

// --- statToObject ---

func TestStatToObject_BasicFields(t *testing.T) {
	info := miniosdk.ObjectInfo{
		Key:          "user1/src1/2026/06/07/item1",
		ContentType:  "text/plain",
		Size:         42,
		ETag:         "abc123",
		LastModified: time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
		UserMetadata: map[string]string{
			"source-item-id": "item1",
			"source-id":      "src1",
			"user-id":        "user1",
			"platform":       "twitter",
			"retention":      "raw_snapshot",
			"original-url":   "https://example.com/1",
		},
	}

	obj := statToObject("user1/src1/2026/06/07/item1", info)

	if obj.Key != "user1/src1/2026/06/07/item1" {
		t.Errorf("Key = %q, want %q", obj.Key, "user1/src1/2026/06/07/item1")
	}
	if obj.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want %q", obj.ContentType, "text/plain")
	}
	if obj.Size != 42 {
		t.Errorf("Size = %d, want 42", obj.Size)
	}
	if obj.ETag != "abc123" {
		t.Errorf("ETag = %q, want %q", obj.ETag, "abc123")
	}
}

func TestStatToObject_Metadata(t *testing.T) {
	info := miniosdk.ObjectInfo{
		UserMetadata: map[string]string{
			"source-item-id": "item-99",
			"source-id":      "src-88",
			"user-id":        "user-77",
			"platform":       "weibo",
			"retention":      "derived",
			"original-url":   "https://weibo.com/post/1",
		},
	}

	obj := statToObject("key", info)

	if obj.Metadata.SourceItemID != "item-99" {
		t.Errorf("SourceItemID = %q, want %q", obj.Metadata.SourceItemID, "item-99")
	}
	if obj.Metadata.SourceID != "src-88" {
		t.Errorf("SourceID = %q, want %q", obj.Metadata.SourceID, "src-88")
	}
	if obj.Metadata.UserID != "user-77" {
		t.Errorf("UserID = %q, want %q", obj.Metadata.UserID, "user-77")
	}
	if obj.Metadata.Platform != "weibo" {
		t.Errorf("Platform = %q, want %q", obj.Metadata.Platform, "weibo")
	}
	if obj.Metadata.Retention != objectstorage.RetentionDerived {
		t.Errorf("Retention = %q, want %q", obj.Metadata.Retention, objectstorage.RetentionDerived)
	}
	if obj.Metadata.OriginalURL != "https://weibo.com/post/1" {
		t.Errorf("OriginalURL = %q, want %q", obj.Metadata.OriginalURL, "https://weibo.com/post/1")
	}
}

func TestStatToObject_ExpiresAt_Present(t *testing.T) {
	info := miniosdk.ObjectInfo{
		UserMetadata: map[string]string{
			"expires-at": "2026-07-07T00:00:00Z",
		},
	}

	obj := statToObject("key", info)

	if obj.Metadata.ExpiresAt == nil {
		t.Fatal("ExpiresAt should not be nil when metadata has expires-at")
	}
	want := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	if !obj.Metadata.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", obj.Metadata.ExpiresAt, want)
	}
}

func TestStatToObject_ExpiresAt_Absent(t *testing.T) {
	info := miniosdk.ObjectInfo{
		UserMetadata: map[string]string{},
	}

	obj := statToObject("key", info)

	if obj.Metadata.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil when metadata has no expires-at", obj.Metadata.ExpiresAt)
	}
}

func TestStatToObject_ExpiresAt_InvalidFormat(t *testing.T) {
	info := miniosdk.ObjectInfo{
		UserMetadata: map[string]string{
			"expires-at": "not-a-date",
		},
	}

	obj := statToObject("key", info)

	if obj.Metadata.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil for invalid date format", obj.Metadata.ExpiresAt)
	}
}

func TestStatToObject_EmptyMetadata(t *testing.T) {
	info := miniosdk.ObjectInfo{}

	obj := statToObject("key", info)

	if obj.Metadata.SourceItemID != "" {
		t.Errorf("SourceItemID = %q, want empty", obj.Metadata.SourceItemID)
	}
	if obj.Metadata.ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil", obj.Metadata.ExpiresAt)
	}
}
