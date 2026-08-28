package minio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	miniosdk "github.com/minio/minio-go/v7"
)

func TestStoreReadsBoundedVaultRevisionSnapshot(t *testing.T) {
	content := "protected Vault revision"
	store := &Store{
		bucket: "hotkey",
		statObject: func(context.Context, string, string, miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error) {
			return miniosdk.ObjectInfo{Key: "knowledge/v1/17/3.md", Size: int64(len(content))}, nil
		},
		readObject: func(context.Context, string, string, miniosdk.GetObjectOptions) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(content)), nil
		},
	}
	got, err := store.ReadVaultSnapshot(context.Background(), "knowledge/v1/17/3.md", 1024)
	if err != nil || got != content {
		t.Fatalf("ReadVaultSnapshot() = %q/%v", got, err)
	}
}

func TestStoreRejectsMissingOversizedOrUnscopedVaultSnapshot(t *testing.T) {
	read := false
	store := &Store{
		bucket: "hotkey",
		statObject: func(_ context.Context, _ string, key string, _ miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error) {
			if strings.Contains(key, "/18/") {
				return miniosdk.ObjectInfo{}, miniosdk.ErrorResponse{Code: "NoSuchKey"}
			}
			return miniosdk.ObjectInfo{Key: key, Size: 2048}, nil
		},
		readObject: func(context.Context, string, string, miniosdk.GetObjectOptions) (io.ReadCloser, error) {
			read = true
			return io.NopCloser(strings.NewReader("unexpected")), nil
		},
	}
	if _, err := store.ReadVaultSnapshot(context.Background(), "knowledge/v1/18/1.md", 1024); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing snapshot error = %v", err)
	}
	if _, err := store.ReadVaultSnapshot(context.Background(), "knowledge/v1/17/3.md", 1024); err == nil || read {
		t.Fatalf("oversized snapshot error/read = %v/%v", err, read)
	}
	if _, err := store.ReadVaultSnapshot(context.Background(), "../secret", 1024); err == nil {
		t.Fatal("unscoped snapshot error = nil")
	}
}

func TestStorePutSnapshotIsImmutableAndIdenticalRetryIsIdempotent(t *testing.T) {
	var stored string
	store := &Store{bucket: "hotkey"}
	store.putObject = func(_ context.Context, _, key string, reader io.Reader, _ int64, options miniosdk.PutObjectOptions) (miniosdk.UploadInfo, error) {
		if options.Header().Get("If-None-Match") != "*" {
			t.Fatal("snapshot PUT does not use If-None-Match: *")
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			return miniosdk.UploadInfo{}, err
		}
		if stored != "" {
			return miniosdk.UploadInfo{}, miniosdk.ErrorResponse{Code: "PreconditionFailed", StatusCode: http.StatusPreconditionFailed}
		}
		stored = string(body)
		return miniosdk.UploadInfo{Key: key, Size: int64(len(body))}, nil
	}
	store.statObject = func(_ context.Context, _, key string, _ miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error) {
		return miniosdk.ObjectInfo{Key: key, Size: int64(len(stored))}, nil
	}
	store.readObject = func(context.Context, string, string, miniosdk.GetObjectOptions) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(stored)), nil
	}

	key := "knowledge/v1/17/3.md"
	if err := store.Put(context.Background(), key, "protected"); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), key, "protected"); err != nil {
		t.Fatalf("identical retry error = %v", err)
	}
	if err := store.Put(context.Background(), key, "conflicting"); !errors.Is(err, domain.ErrVaultConflict) {
		t.Fatalf("conflicting retry error = %v", err)
	}
	if stored != "protected" {
		t.Fatalf("stored snapshot = %q", stored)
	}
}
