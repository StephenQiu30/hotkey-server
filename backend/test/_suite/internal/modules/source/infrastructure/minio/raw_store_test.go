package minio

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	miniosdk "github.com/minio/minio-go/v7"
)

type rawObjectClientFake struct {
	objects map[string]rawObjectFixture
	puts    int
}

type rawObjectFixture struct {
	payload  []byte
	mimeType string
	metadata map[string]string
}

func (client *rawObjectClientFake) StatObject(_ context.Context, bucket, objectKey string, _ miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error) {
	fixture, found := client.objects[bucket+"/"+objectKey]
	if !found {
		return miniosdk.ObjectInfo{}, miniosdk.ErrorResponse{Code: "NoSuchKey", StatusCode: http.StatusNotFound}
	}
	metadata := make(http.Header, len(fixture.metadata)+1)
	metadata.Set("Content-Type", fixture.mimeType)
	for name, value := range fixture.metadata {
		metadata.Set("X-Amz-Meta-"+name, value)
	}
	return miniosdk.ObjectInfo{Key: objectKey, Size: int64(len(fixture.payload)), ContentType: fixture.mimeType, Metadata: metadata}, nil
}

func (client *rawObjectClientFake) PutObject(_ context.Context, bucket, objectKey string, reader io.Reader, size int64, options miniosdk.PutObjectOptions) (miniosdk.UploadInfo, error) {
	client.puts++
	payload, err := io.ReadAll(reader)
	if err != nil {
		return miniosdk.UploadInfo{}, err
	}
	if int64(len(payload)) != size {
		return miniosdk.UploadInfo{}, errors.New("size mismatch")
	}
	if client.objects == nil {
		client.objects = make(map[string]rawObjectFixture)
	}
	identity := bucket + "/" + objectKey
	if _, exists := client.objects[identity]; exists {
		return miniosdk.UploadInfo{}, miniosdk.ErrorResponse{Code: "PreconditionFailed", StatusCode: http.StatusPreconditionFailed}
	}
	metadata := make(map[string]string, len(options.UserMetadata))
	for name, value := range options.UserMetadata {
		metadata[name] = value
	}
	client.objects[identity] = rawObjectFixture{payload: append([]byte(nil), payload...), mimeType: options.ContentType, metadata: metadata}
	return miniosdk.UploadInfo{Key: objectKey, Size: size}, nil
}

func TestRawEvidenceStorePutIfAbsentIsSourceScopedAndIdempotent(t *testing.T) {
	t.Parallel()

	client := &rawObjectClientFake{}
	store := newRawEvidenceStore(client, "evidence")
	object := rawStoreObject(42, "raw response")
	first, err := store.PutIfAbsent(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.PutIfAbsent(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || client.puts != 1 || len(client.objects) != 1 {
		t.Fatalf("receipts=%#v/%#v puts=%d objects=%d", first, second, client.puts, len(client.objects))
	}
	if !strings.HasPrefix(first.ObjectKey, "source-raw/v1/42/") || first.ObjectKey != application.RawEvidenceObjectKey(42, object.EvidenceKey) {
		t.Fatalf("object key = %q", first.ObjectKey)
	}

	otherSource := object
	otherSource.SourceConnectionID = 43
	otherSource.ObjectKey = application.RawEvidenceObjectKey(43, object.EvidenceKey)
	if _, err := store.PutIfAbsent(context.Background(), otherSource); err != nil {
		t.Fatal(err)
	}
	if len(client.objects) != 2 {
		t.Fatalf("source-scoped object identity collapsed: %#v", client.objects)
	}
}

func TestRawEvidenceStoreRejectsSameKeyDifferentContentOrMetadata(t *testing.T) {
	t.Parallel()

	client := &rawObjectClientFake{}
	store := newRawEvidenceStore(client, "evidence")
	object := rawStoreObject(42, "raw response")
	if _, err := store.PutIfAbsent(context.Background(), object); err != nil {
		t.Fatal(err)
	}

	identity := "evidence/" + object.ObjectKey
	fixture := client.objects[identity]
	fixture.payload = []byte("different bytes")
	client.objects[identity] = fixture
	if _, err := store.PutIfAbsent(context.Background(), object); !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("PutIfAbsent() error = %v, want conflict", err)
	}

	fixture = client.objects[identity]
	fixture.payload = append([]byte(nil), object.Payload...)
	fixture.metadata[rawMetadataPayloadSHA256] = fmt.Sprintf("%064d", 0)
	client.objects[identity] = fixture
	if _, err := store.PutIfAbsent(context.Background(), object); !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("metadata conflict error = %v", err)
	}
}

func TestRawEvidenceStoreValidatesDeclaredPayloadBeforeClientCall(t *testing.T) {
	t.Parallel()

	client := &rawObjectClientFake{}
	store := newRawEvidenceStore(client, "evidence")
	object := rawStoreObject(42, "raw response")
	object.Payload = []byte("tampered")
	if _, err := store.PutIfAbsent(context.Background(), object); err == nil {
		t.Fatal("PutIfAbsent() accepted payload that does not match declared SHA-256")
	}
	if client.puts != 0 {
		t.Fatalf("invalid object reached MinIO client: %d puts", client.puts)
	}
}

func rawStoreObject(sourceConnectionID int64, value string) application.StoreRawEvidenceCommand {
	payload := []byte(value)
	payloadDigest := fmt.Sprintf("%x", sha256.Sum256(payload))
	profile, err := domain.NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if err != nil {
		panic(err)
	}
	evidenceKey, err := domain.EvidenceSnapshotIdentity(payloadDigest, profile)
	if err != nil {
		panic(err)
	}
	return application.StoreRawEvidenceCommand{
		SourceConnectionID: sourceConnectionID, EvidenceKey: evidenceKey,
		ObjectKey: application.RawEvidenceObjectKey(sourceConnectionID, evidenceKey),
		Payload:   payload, PayloadSHA256: payloadDigest, CollectorProfileVersion: profile.String(),
		MIMEType: "application/atom+xml; charset=utf-8",
	}
}
