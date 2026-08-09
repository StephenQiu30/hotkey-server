package minio

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type rawEvidenceReadClient interface {
	StatObject(context.Context, string, string, miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error)
	ReadObject(context.Context, string, string, miniosdk.GetObjectOptions) (io.ReadCloser, error)
}

type minioRawEvidenceReadClient struct {
	client *miniosdk.Client
}

func (client minioRawEvidenceReadClient) StatObject(ctx context.Context, bucket, objectKey string, options miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error) {
	return client.client.StatObject(ctx, bucket, objectKey, options)
}

func (client minioRawEvidenceReadClient) ReadObject(ctx context.Context, bucket, objectKey string, options miniosdk.GetObjectOptions) (io.ReadCloser, error) {
	return client.client.GetObject(ctx, bucket, objectKey, options)
}

// RawEvidenceObjectReader is Source's MinIO read adapter. Object identity is
// accepted only from the Source Application manifest port and never appears
// in the returned Application result.
type RawEvidenceObjectReader struct {
	client rawEvidenceReadClient
	bucket string
}

var _ application.RawEvidenceObjectReader = (*RawEvidenceObjectReader)(nil)

func NewRawEvidenceObjectReader(cfg config.MinIOConfig) (*RawEvidenceObjectReader, error) {
	if err := cfg.ValidateRuntime(); err != nil {
		return nil, fmt.Errorf("invalid MinIO configuration: %w", err)
	}
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL,
		Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1,
	})
	if err != nil {
		return nil, errors.New("create Source raw evidence MinIO reader")
	}
	return newRawEvidenceObjectReader(minioRawEvidenceReadClient{client: client}, cfg.Bucket), nil
}

func newRawEvidenceObjectReader(client rawEvidenceReadClient, bucket string) *RawEvidenceObjectReader {
	return &RawEvidenceObjectReader{client: client, bucket: bucket}
}

// rawEvidenceObjectReadRecord is the MinIO-private result of HEAD/GET/HEAD.
// The public mapper copies only verified payload bytes.
type rawEvidenceObjectReadRecord struct {
	manifest rawObjectRecord
	payload  []byte
}

func (record rawEvidenceObjectReadRecord) applicationResult() application.ReadRawEvidenceObjectResult {
	return application.ReadRawEvidenceObjectResult{Payload: append([]byte(nil), record.payload...)}
}

func (reader *RawEvidenceObjectReader) Read(ctx context.Context, query application.ReadRawEvidenceObjectQuery) (application.ReadRawEvidenceObjectResult, error) {
	if reader == nil || reader.client == nil || reader.bucket == "" {
		return application.ReadRawEvidenceObjectResult{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return application.ReadRawEvidenceObjectResult{}, fmt.Errorf("%w: invalid raw evidence object query", sharedrepository.ErrInvalidInput)
	}

	before, err := reader.readManifest(ctx, query.ObjectKey)
	if err != nil {
		return application.ReadRawEvidenceObjectResult{}, err
	}
	if !rawEvidenceReadManifestMatches(before, query) {
		return application.ReadRawEvidenceObjectResult{}, domain.ErrRawEvidenceConflict
	}

	stream, err := reader.client.ReadObject(ctx, reader.bucket, query.ObjectKey, miniosdk.GetObjectOptions{})
	if err != nil {
		return application.ReadRawEvidenceObjectResult{}, mapRawEvidenceReadError(err)
	}
	if stream == nil {
		return application.ReadRawEvidenceObjectResult{}, sharedrepository.ErrUnavailable
	}
	payload, readErr := io.ReadAll(io.LimitReader(stream, query.MaximumBytes+1))
	closeErr := stream.Close()
	if readErr != nil {
		return application.ReadRawEvidenceObjectResult{}, mapRawEvidenceReadError(readErr)
	}
	if closeErr != nil {
		return application.ReadRawEvidenceObjectResult{}, mapRawEvidenceReadError(closeErr)
	}
	if int64(len(payload)) != query.SizeBytes || int64(len(payload)) > query.MaximumBytes || !rawEvidencePayloadDigestMatches(payload, query.PayloadSHA256) {
		return application.ReadRawEvidenceObjectResult{}, domain.ErrRawEvidenceConflict
	}

	after, err := reader.readManifest(ctx, query.ObjectKey)
	if err != nil {
		return application.ReadRawEvidenceObjectResult{}, err
	}
	if !rawEvidenceReadManifestMatches(after, query) || before != after {
		return application.ReadRawEvidenceObjectResult{}, domain.ErrRawEvidenceConflict
	}
	return (rawEvidenceObjectReadRecord{manifest: after, payload: payload}).applicationResult(), nil
}

func (reader *RawEvidenceObjectReader) readManifest(ctx context.Context, objectKey string) (rawObjectRecord, error) {
	object, err := reader.client.StatObject(ctx, reader.bucket, objectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		return rawObjectRecord{}, mapRawEvidenceReadError(err)
	}
	if object.Key != "" && object.Key != objectKey {
		return rawObjectRecord{}, domain.ErrRawEvidenceConflict
	}
	record, err := rawObjectRecordFromInfo(objectKey, object)
	if err != nil {
		return rawObjectRecord{}, domain.ErrRawEvidenceConflict
	}
	return record, nil
}

func rawEvidenceReadManifestMatches(record rawObjectRecord, query application.ReadRawEvidenceObjectQuery) bool {
	return record.SourceConnectionID == query.SourceConnectionID && record.EvidenceKey == query.EvidenceKey &&
		record.ObjectKey == query.ObjectKey && record.PayloadSHA256 == query.PayloadSHA256 &&
		record.CollectorProfileVersion == query.CollectorProfileVersion && record.MIMEType == query.MIMEType &&
		record.SizeBytes == query.SizeBytes
}

func rawEvidencePayloadDigestMatches(payload []byte, expected string) bool {
	declared, err := hex.DecodeString(expected)
	if err != nil || len(declared) != sha256.Size {
		return false
	}
	digest := sha256.Sum256(payload)
	return subtle.ConstantTimeCompare(digest[:], declared) == 1
}

func mapRawEvidenceReadError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case rawObjectMissing(err):
		return sharedrepository.ErrNotFound
	default:
		return sharedrepository.ErrUnavailable
	}
}
