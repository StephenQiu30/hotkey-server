// Package minio implements Source-owned immutable raw response storage.
package minio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	rawMetadataPayloadSHA256           = "payload-sha256"
	rawMetadataEvidenceKey             = "evidence-key"
	rawMetadataSourceConnectionID      = "source-connection-id"
	rawMetadataCollectorProfileVersion = "collector-profile-version"
)

type rawObjectClient interface {
	StatObject(context.Context, string, string, miniosdk.StatObjectOptions) (miniosdk.ObjectInfo, error)
	PutObject(context.Context, string, string, io.Reader, int64, miniosdk.PutObjectOptions) (miniosdk.UploadInfo, error)
}

// rawObjectManifest is the MinIO-specific write record. It remains private so
// SDK metadata cannot cross the Infrastructure boundary.
type rawObjectManifest struct {
	SourceConnectionID      int64
	EvidenceKey             string
	ObjectKey               string
	Payload                 []byte
	PayloadSHA256           string
	CollectorProfileVersion string
	MIMEType                string
}

func rawObjectManifestFromCommand(command application.StoreRawEvidenceCommand) (rawObjectManifest, error) {
	if err := command.Validate(); err != nil {
		return rawObjectManifest{}, err
	}
	return rawObjectManifest{
		SourceConnectionID: command.SourceConnectionID, EvidenceKey: command.EvidenceKey, ObjectKey: command.ObjectKey,
		Payload: append([]byte(nil), command.Payload...), PayloadSHA256: command.PayloadSHA256,
		CollectorProfileVersion: command.CollectorProfileVersion, MIMEType: command.MIMEType,
	}, nil
}

func (manifest rawObjectManifest) userMetadata() map[string]string {
	return map[string]string{
		rawMetadataPayloadSHA256:           manifest.PayloadSHA256,
		rawMetadataEvidenceKey:             manifest.EvidenceKey,
		rawMetadataSourceConnectionID:      strconv.FormatInt(manifest.SourceConnectionID, 10),
		rawMetadataCollectorProfileVersion: manifest.CollectorProfileVersion,
	}
}

// rawObjectRecord is the MinIO-specific HEAD projection.
type rawObjectRecord struct {
	SourceConnectionID      int64
	EvidenceKey             string
	ObjectKey               string
	PayloadSHA256           string
	CollectorProfileVersion string
	MIMEType                string
	SizeBytes               int64
}

func rawObjectRecordFromInfo(objectKey string, object miniosdk.ObjectInfo) (rawObjectRecord, error) {
	sourceConnectionID, err := strconv.ParseInt(object.Metadata.Get("X-Amz-Meta-"+rawMetadataSourceConnectionID), 10, 64)
	if err != nil || sourceConnectionID <= 0 {
		return rawObjectRecord{}, domain.ErrRawEvidenceConflict
	}
	mimeType := object.ContentType
	if mimeType == "" {
		mimeType = object.Metadata.Get("Content-Type")
	}
	return rawObjectRecord{
		SourceConnectionID: sourceConnectionID,
		EvidenceKey:        object.Metadata.Get("X-Amz-Meta-" + rawMetadataEvidenceKey), ObjectKey: objectKey,
		PayloadSHA256:           object.Metadata.Get("X-Amz-Meta-" + rawMetadataPayloadSHA256),
		CollectorProfileVersion: object.Metadata.Get("X-Amz-Meta-" + rawMetadataCollectorProfileVersion),
		MIMEType:                mimeType, SizeBytes: object.Size,
	}, nil
}

func rawEvidenceResultFromRecord(record rawObjectRecord) application.StoreRawEvidenceResult {
	return application.StoreRawEvidenceResult{
		SourceConnectionID: record.SourceConnectionID, EvidenceKey: record.EvidenceKey, ObjectKey: record.ObjectKey,
		PayloadSHA256: record.PayloadSHA256, CollectorProfileVersion: record.CollectorProfileVersion,
		MIMEType: record.MIMEType, SizeBytes: record.SizeBytes,
	}
}

type RawEvidenceStore struct {
	client rawObjectClient
	bucket string
}

func NewRawEvidenceStore(cfg config.MinIOConfig) (*RawEvidenceStore, error) {
	if err := cfg.ValidateRuntime(); err != nil {
		return nil, fmt.Errorf("invalid MinIO configuration: %w", err)
	}
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL,
		Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("create Source raw MinIO client: %w", err)
	}
	return newRawEvidenceStore(client, cfg.Bucket), nil
}

func newRawEvidenceStore(client rawObjectClient, bucket string) *RawEvidenceStore {
	return &RawEvidenceStore{client: client, bucket: bucket}
}

// PutIfAbsent maps the Application command to a private object manifest, uses
// an If-None-Match write, and maps the private HEAD record back to a Result.
func (store *RawEvidenceStore) PutIfAbsent(ctx context.Context, command application.StoreRawEvidenceCommand) (application.StoreRawEvidenceResult, error) {
	if store == nil || store.client == nil || store.bucket == "" {
		return application.StoreRawEvidenceResult{}, errors.New("Source raw evidence store is not initialized")
	}
	manifest, err := rawObjectManifestFromCommand(command)
	if err != nil {
		return application.StoreRawEvidenceResult{}, err
	}
	if result, err := store.result(ctx, manifest.ObjectKey); err == nil {
		if err := result.ValidateAgainst(command); err != nil {
			return application.StoreRawEvidenceResult{}, domain.ErrRawEvidenceConflict
		}
		return result, nil
	} else if !rawObjectMissing(err) {
		return application.StoreRawEvidenceResult{}, err
	}

	options := miniosdk.PutObjectOptions{ContentType: manifest.MIMEType, UserMetadata: manifest.userMetadata()}
	options.SetMatchETagExcept("*")
	if _, err := store.client.PutObject(ctx, store.bucket, manifest.ObjectKey, bytes.NewReader(manifest.Payload), int64(len(manifest.Payload)), options); err != nil {
		if rawObjectPreconditionFailed(err) {
			result, statErr := store.result(ctx, manifest.ObjectKey)
			if statErr == nil && result.ValidateAgainst(command) == nil {
				return result, nil
			}
			return application.StoreRawEvidenceResult{}, domain.ErrRawEvidenceConflict
		}
		return application.StoreRawEvidenceResult{}, fmt.Errorf("put Source raw evidence object: %w", err)
	}
	result, err := store.result(ctx, manifest.ObjectKey)
	if err != nil {
		return application.StoreRawEvidenceResult{}, err
	}
	if err := result.ValidateAgainst(command); err != nil {
		return application.StoreRawEvidenceResult{}, domain.ErrRawEvidenceConflict
	}
	return result, nil
}

func (store *RawEvidenceStore) result(ctx context.Context, objectKey string) (application.StoreRawEvidenceResult, error) {
	object, err := store.client.StatObject(ctx, store.bucket, objectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		return application.StoreRawEvidenceResult{}, fmt.Errorf("head Source raw evidence object: %w", err)
	}
	record, err := rawObjectRecordFromInfo(objectKey, object)
	if err != nil {
		return application.StoreRawEvidenceResult{}, err
	}
	return rawEvidenceResultFromRecord(record), nil
}

func rawObjectMissing(err error) bool {
	var response miniosdk.ErrorResponse
	return errors.As(err, &response) && (response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NoSuchObject")
}

func rawObjectPreconditionFailed(err error) bool {
	var response miniosdk.ErrorResponse
	return errors.As(err, &response) && (response.StatusCode == 412 || response.Code == "PreconditionFailed")
}
