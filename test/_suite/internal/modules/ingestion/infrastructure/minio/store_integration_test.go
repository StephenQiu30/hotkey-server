//go:build integration

package minio_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionminio "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/minio"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestStoreIntegrationPersistsVerifiedDeterministicEvidence(t *testing.T) {
	cfg := integrationMinIOConfig(t)
	client := integrationClient(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ensureIntegrationBucket(t, ctx, client, cfg.Bucket)

	store, err := ingestionminio.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	sourceID := time.Now().UTC().UnixNano()
	prefix := fmt.Sprintf("evidence/v1/%d/", sourceID)
	t.Cleanup(func() { cleanupIntegrationPrefix(t, store, prefix) })

	text := "real MinIO evidence body"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
	object := ingestiondomain.EvidenceObject{
		SourceConnectionID: sourceID,
		ObjectKey:          ingestionminio.EvidenceObjectKey(sourceID, digest),
		Text:               text,
		SHA256:             digest,
	}
	first, err := store.PutText(ctx, object)
	if err != nil {
		t.Fatalf("PutText(first) error = %v", err)
	}
	second, err := store.PutText(ctx, object)
	if err != nil {
		t.Fatalf("PutText(retry) error = %v", err)
	}
	if first != second {
		t.Fatalf("PutText receipts = %#v and %#v, want deterministic reuse", first, second)
	}
	if first.ObjectKey != object.ObjectKey || first.SHA256 != digest || first.SizeBytes != int64(len(text)) {
		t.Fatalf("PutText receipt = %#v, want verified source evidence", first)
	}

	head, err := client.StatObject(ctx, cfg.Bucket, object.ObjectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		t.Fatalf("StatObject() error = %v", err)
	}
	if head.Size != int64(len(text)) || head.Metadata.Get("X-Amz-Meta-Sha256") != digest {
		t.Fatalf("Head metadata/size = %#v/%d, want %s/%d", head.Metadata, head.Size, digest, len(text))
	}

	receipts, err := store.ListPrefix(ctx, prefix)
	if err != nil {
		t.Fatalf("ListPrefix() error = %v", err)
	}
	if len(receipts) != 1 || receipts[0] != first {
		t.Fatalf("ListPrefix() = %#v, want one verified receipt %#v", receipts, first)
	}
}

func integrationMinIOConfig(t *testing.T) config.MinIOConfig {
	t.Helper()
	config := config.MinIOConfig{
		Endpoint:  os.Getenv("HOTKEY_TEST_MINIO_ENDPOINT"),
		AccessKey: os.Getenv("HOTKEY_TEST_MINIO_ACCESS_KEY"),
		SecretKey: os.Getenv("HOTKEY_TEST_MINIO_SECRET_KEY"),
		Bucket:    os.Getenv("HOTKEY_TEST_MINIO_BUCKET"),
	}
	if err := config.ValidateRuntime(); err != nil {
		t.Fatalf("integration MinIO configuration is required: %v", err)
	}
	return config
}

func integrationClient(t *testing.T, cfg config.MinIOConfig) *miniosdk.Client {
	t.Helper()
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.UseSSL,
		Region:       "us-east-1",
		BucketLookup: miniosdk.BucketLookupPath,
		MaxRetries:   1,
	})
	if err != nil {
		t.Fatalf("create integration MinIO client: %v", err)
	}
	return client
}

func ensureIntegrationBucket(t *testing.T, ctx context.Context, client *miniosdk.Client, bucket string) {
	t.Helper()
	err := client.MakeBucket(ctx, bucket, miniosdk.MakeBucketOptions{Region: "us-east-1"})
	if err == nil {
		return
	}
	response := miniosdk.ToErrorResponse(err)
	if response.Code != "BucketAlreadyOwnedByYou" && response.Code != "BucketAlreadyExists" {
		t.Fatalf("MakeBucket(%q) error = %v", bucket, err)
	}
}

func cleanupIntegrationPrefix(t *testing.T, store *ingestionminio.Store, prefix string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	receipts, err := store.ListPrefix(ctx, prefix)
	if err != nil {
		t.Errorf("ListPrefix cleanup error = %v", err)
		return
	}
	for _, receipt := range receipts {
		if err := store.Delete(ctx, receipt.ObjectKey); err != nil {
			t.Errorf("Delete(%q) cleanup error = %v", receipt.ObjectKey, err)
		}
	}
}

func TestIntegrationMinIOConfigurationErrorDoesNotContainSecret(t *testing.T) {
	cfg := config.MinIOConfig{Endpoint: "127.0.0.1:19007", AccessKey: "fixture", SecretKey: "integration-secret", Bucket: ""}
	err := cfg.ValidateRuntime()
	if err == nil || strings.Contains(err.Error(), cfg.SecretKey) {
		t.Fatalf("ValidateRuntime() error = %v, want safe bucket validation", err)
	}
}
