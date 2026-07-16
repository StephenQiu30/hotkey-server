// Package minio implements ingestion's single evidence-object boundary.
package minio

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const evidenceRootPrefix = "evidence/v1/"

// Store owns one configured MinIO client and only accepts deterministic,
// ingestion-owned evidence keys. Provider types stay inside this package.
type Store struct {
	client *miniosdk.Client
	bucket string
}

// NewStore creates the ingestion evidence adapter. The supplied configuration
// is validated without including credentials in any returned error.
func NewStore(cfg config.MinIOConfig) (*Store, error) {
	if err := cfg.ValidateRuntime(); err != nil {
		return nil, fmt.Errorf("invalid MinIO configuration: %w", err)
	}
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.UseSSL,
		Region:       "us-east-1",
		BucketLookup: miniosdk.BucketLookupPath,
		MaxRetries:   1,
	})
	if err != nil {
		return nil, fmt.Errorf("create MinIO client: %w", err)
	}
	return &Store{client: client, bucket: cfg.Bucket}, nil
}

// EvidenceObjectKey returns the only object key layout accepted by Store.
// Invalid inputs return an empty string so callers cannot accidentally write
// a malformed key into another object's namespace.
func EvidenceObjectKey(sourceID int64, digest string) string {
	if sourceID <= 0 || !validSHA256(digest) {
		return ""
	}
	return fmt.Sprintf("%s%d/%s/%s.txt", evidenceRootPrefix, sourceID, digest[:2], digest)
}

// PutText stores body text only after its declared digest is proven correct.
// It reuses an existing object only if a Head check proves the same digest and
// size; otherwise it never silently treats the conflicting remote object as
// valid evidence.
func (store *Store) PutText(ctx context.Context, object ingestiondomain.EvidenceObject) (ingestiondomain.EvidenceReceipt, error) {
	if err := validateEvidenceObject(object); err != nil {
		return ingestiondomain.EvidenceReceipt{}, err
	}
	if receipt, err := store.receipt(ctx, object.ObjectKey); err == nil {
		if receipt.SHA256 != object.SHA256 || receipt.SizeBytes != int64(len(object.Text)) {
			return ingestiondomain.EvidenceReceipt{}, fmt.Errorf("existing evidence object does not match declared SHA-256 or size")
		}
		return receipt, nil
	} else if !missingObject(err) {
		return ingestiondomain.EvidenceReceipt{}, err
	}

	if _, err := store.client.PutObject(ctx, store.bucket, object.ObjectKey, strings.NewReader(object.Text), int64(len(object.Text)), miniosdk.PutObjectOptions{
		ContentType:  "text/plain; charset=utf-8",
		UserMetadata: map[string]string{"sha256": object.SHA256},
	}); err != nil {
		return ingestiondomain.EvidenceReceipt{}, fmt.Errorf("put evidence object: %w", err)
	}

	receipt, err := store.receipt(ctx, object.ObjectKey)
	if err != nil {
		return ingestiondomain.EvidenceReceipt{}, err
	}
	if receipt.SHA256 != object.SHA256 || receipt.SizeBytes != int64(len(object.Text)) {
		return ingestiondomain.EvidenceReceipt{}, fmt.Errorf("stored evidence object does not match declared SHA-256 or size")
	}
	return receipt, nil
}

// Delete removes only a deterministic ingestion evidence object.
func (store *Store) Delete(ctx context.Context, objectKey string) error {
	if !validEvidenceKey(objectKey) {
		return fmt.Errorf("invalid evidence object key")
	}
	if err := store.client.RemoveObject(ctx, store.bucket, objectKey, miniosdk.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete evidence object: %w", err)
	}
	return nil
}

// ListPrefix lists only the ingestion evidence namespace. Each object is
// subsequently headed so callers receive verified SHA-256 metadata and size.
func (store *Store) ListPrefix(ctx context.Context, prefix string) ([]ingestiondomain.EvidenceReceipt, error) {
	if !validEvidencePrefix(prefix) {
		return nil, fmt.Errorf("invalid evidence prefix")
	}
	receipts := make([]ingestiondomain.EvidenceReceipt, 0)
	for object := range store.client.ListObjects(ctx, store.bucket, miniosdk.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if object.Err != nil {
			return nil, fmt.Errorf("list evidence objects: %w", object.Err)
		}
		if !validEvidenceKey(object.Key) {
			return nil, fmt.Errorf("listed object outside deterministic evidence key contract")
		}
		receipt, err := store.receipt(ctx, object.Key)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func (store *Store) receipt(ctx context.Context, objectKey string) (ingestiondomain.EvidenceReceipt, error) {
	object, err := store.client.StatObject(ctx, store.bucket, objectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		return ingestiondomain.EvidenceReceipt{}, fmt.Errorf("head evidence object: %w", err)
	}
	digest := object.Metadata.Get("X-Amz-Meta-Sha256")
	if !validSHA256(digest) {
		return ingestiondomain.EvidenceReceipt{}, fmt.Errorf("evidence object is missing valid SHA-256 metadata")
	}
	return ingestiondomain.EvidenceReceipt{ObjectKey: objectKey, SHA256: digest, SizeBytes: object.Size}, nil
}

func validateEvidenceObject(object ingestiondomain.EvidenceObject) error {
	if object.SourceConnectionID <= 0 {
		return fmt.Errorf("source connection id is required")
	}
	if object.Text == "" {
		return fmt.Errorf("evidence text must not be empty")
	}
	if !validSHA256(object.SHA256) {
		return fmt.Errorf("evidence SHA-256 must be lowercase hexadecimal")
	}
	computed := fmt.Sprintf("%x", sha256.Sum256([]byte(object.Text)))
	if object.SHA256 != computed {
		return fmt.Errorf("evidence SHA-256 does not match text")
	}
	if want := EvidenceObjectKey(object.SourceConnectionID, object.SHA256); object.ObjectKey != want {
		return fmt.Errorf("evidence object key must be source-scoped and deterministic")
	}
	return nil
}

func missingObject(err error) bool {
	var response miniosdk.ErrorResponse
	if errors.As(err, &response) {
		return response.StatusCode == 404 || response.Code == "NoSuchKey"
	}
	return false
}

func validEvidencePrefix(prefix string) bool {
	if prefix == evidenceRootPrefix {
		return true
	}
	if !strings.HasPrefix(prefix, evidenceRootPrefix) || !strings.HasSuffix(prefix, "/") {
		return false
	}
	sourceID := strings.TrimSuffix(strings.TrimPrefix(prefix, evidenceRootPrefix), "/")
	if sourceID == "" || strings.Contains(sourceID, "/") {
		return false
	}
	parsed, err := strconv.ParseInt(sourceID, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == sourceID
}

func validEvidenceKey(objectKey string) bool {
	if !strings.HasPrefix(objectKey, evidenceRootPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(objectKey, evidenceRootPrefix), "/")
	if len(parts) != 3 {
		return false
	}
	sourceID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || sourceID <= 0 || strconv.FormatInt(sourceID, 10) != parts[0] {
		return false
	}
	if !strings.HasSuffix(parts[2], ".txt") {
		return false
	}
	digest := strings.TrimSuffix(parts[2], ".txt")
	return validSHA256(digest) && parts[1] == digest[:2] && objectKey == EvidenceObjectKey(sourceID, digest)
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
