package minio

import (
	"context"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/application"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const root = "knowledge/v1/"

type Store struct {
	client *miniosdk.Client
	bucket string
}

func NewStore(cfg config.MinIOConfig) (*Store, error) {
	if err := cfg.ValidateRuntime(); err != nil {
		return nil, fmt.Errorf("invalid knowledge snapshot configuration: %w", err)
	}
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL, Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1})
	if err != nil {
		return nil, fmt.Errorf("create knowledge snapshot client: %w", err)
	}
	return &Store{client: client, bucket: cfg.Bucket}, nil
}

func ObjectKey(documentID, revision int64) string {
	if documentID <= 0 || revision < 0 {
		return ""
	}
	return fmt.Sprintf("%s%d/%d.md", root, documentID, revision)
}

func (store *Store) Put(ctx context.Context, objectKey, content string) error {
	if store == nil || store.client == nil || !strings.HasPrefix(objectKey, root) || strings.Contains(objectKey[len(root):], "..") || strings.TrimSpace(content) == "" {
		return fmt.Errorf("invalid knowledge snapshot")
	}
	if _, err := store.client.PutObject(ctx, store.bucket, objectKey, strings.NewReader(content), int64(len(content)), miniosdk.PutObjectOptions{ContentType: "text/markdown; charset=utf-8"}); err != nil {
		return fmt.Errorf("put knowledge snapshot: %w", err)
	}
	return nil
}

var _ application.SnapshotStore = (*Store)(nil)
