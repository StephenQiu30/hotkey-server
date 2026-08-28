package minio

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const root = "knowledge/v1/"

type Store struct {
	client     *miniosdk.Client
	bucket     string
	putObject  func(context.Context, string, string, io.Reader, int64, miniosdk.PutObjectOptions) (miniosdk.UploadInfo, error)
	statObject func(context.Context, string, string, miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error)
	readObject func(context.Context, string, string, miniosdk.GetObjectOptions) (io.ReadCloser, error)
}

func NewStore(cfg config.MinIOConfig) (*Store, error) {
	if err := cfg.ValidateRuntime(); err != nil {
		return nil, fmt.Errorf("invalid knowledge snapshot configuration: %w", err)
	}
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL, Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1})
	if err != nil {
		return nil, fmt.Errorf("create knowledge snapshot client: %w", err)
	}
	return &Store{
		client: client, bucket: cfg.Bucket,
		putObject:  client.PutObject,
		statObject: client.StatObject,
		readObject: func(ctx context.Context, bucket, objectKey string, options miniosdk.GetObjectOptions) (io.ReadCloser, error) {
			return client.GetObject(ctx, bucket, objectKey, options)
		},
	}, nil
}

func ObjectKey(documentID, revision int64) string {
	if documentID <= 0 || revision < 0 {
		return ""
	}
	return fmt.Sprintf("%s%d/%d.md", root, documentID, revision)
}

func (store *Store) Put(ctx context.Context, objectKey, content string) error {
	if store == nil || store.putObject == nil || !validSnapshotKey(objectKey) || strings.TrimSpace(content) == "" {
		return fmt.Errorf("invalid knowledge snapshot")
	}
	options := miniosdk.PutObjectOptions{ContentType: "text/markdown; charset=utf-8", DisableMultipart: true}
	options.SetMatchETagExcept("*")
	if _, err := store.putObject(ctx, store.bucket, objectKey, strings.NewReader(content), int64(len(content)), options); err != nil {
		if !snapshotAlreadyExists(err) {
			return fmt.Errorf("put knowledge snapshot: %w", err)
		}
		existing, readErr := store.ReadVaultSnapshot(ctx, objectKey, int64(len(content)))
		if readErr != nil {
			return fmt.Errorf("verify existing knowledge snapshot: %w", readErr)
		}
		if existing != content {
			return domain.ErrVaultConflict
		}
	}
	return nil
}

// ReadVaultSnapshot returns a bounded immutable full-file Revision copy. The
// Application and Domain layers subsequently compare its content hash and
// stable identity with the current PostgreSQL projection before recovery.
func (store *Store) ReadVaultSnapshot(ctx context.Context, objectKey string, maxBytes int64) (string, error) {
	if store == nil || store.bucket == "" || store.statObject == nil || store.readObject == nil || !validSnapshotKey(objectKey) || maxBytes <= 0 {
		return "", fmt.Errorf("invalid knowledge snapshot read")
	}
	info, err := store.statObject(ctx, store.bucket, objectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		return "", mapSnapshotReadError(err)
	}
	if (info.Key != "" && info.Key != objectKey) || info.Size < 0 {
		return "", fmt.Errorf("knowledge snapshot identity conflict")
	}
	if info.Size > maxBytes {
		return "", fmt.Errorf("knowledge snapshot exceeds read limit")
	}
	object, err := store.readObject(ctx, store.bucket, objectKey, miniosdk.GetObjectOptions{})
	if err != nil {
		return "", mapSnapshotReadError(err)
	}
	if object == nil {
		return "", fmt.Errorf("knowledge snapshot reader is unavailable")
	}
	body, readErr := io.ReadAll(io.LimitReader(object, maxBytes+1))
	closeErr := object.Close()
	if readErr != nil {
		return "", fmt.Errorf("read knowledge snapshot: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close knowledge snapshot: %w", closeErr)
	}
	if int64(len(body)) > maxBytes || int64(len(body)) != info.Size {
		return "", fmt.Errorf("knowledge snapshot size conflict")
	}
	return string(body), nil
}

func validSnapshotKey(objectKey string) bool {
	if !strings.HasPrefix(objectKey, root) || strings.Contains(objectKey[len(root):], "..") {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(objectKey[len(root):], ".md"), "/")
	if len(parts) != 2 || !strings.HasSuffix(objectKey, ".md") {
		return false
	}
	documentID, documentErr := strconv.ParseInt(parts[0], 10, 64)
	revision, revisionErr := strconv.ParseInt(parts[1], 10, 64)
	return documentErr == nil && revisionErr == nil && ObjectKey(documentID, revision) == objectKey
}

func mapSnapshotReadError(err error) error {
	response := miniosdk.ToErrorResponse(err)
	if response.Code == "NoSuchKey" || response.Code == "NoSuchObject" || response.Code == "NotFound" {
		return os.ErrNotExist
	}
	return fmt.Errorf("read knowledge snapshot object: %w", err)
}

func snapshotAlreadyExists(err error) bool {
	response := miniosdk.ToErrorResponse(err)
	return response.StatusCode == 412 || response.Code == "PreconditionFailed" || response.Code == "ConditionalRequestConflict"
}

var _ application.SnapshotStore = (*Store)(nil)
var _ application.VaultSnapshotReader = (*Store)(nil)
