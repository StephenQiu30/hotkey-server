package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/StephenQiu30/hotkey-server/internal/domain/objectstorage"
)

type Config struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	Bucket     string
	UseSSL     bool
	Location   string
}

type Client struct {
	inner  *minio.Client
	bucket string
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("minio endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("minio bucket is required")
	}

	inner, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &Client{
		inner:  inner,
		bucket: cfg.Bucket,
	}, nil
}

// EnsureBucket creates the bucket if it doesn't exist.
func (c *Client) EnsureBucket(ctx context.Context, location string) error {
	exists, err := c.inner.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("check bucket existence: %w", err)
	}
	if !exists {
		if location == "" {
			location = "us-east-1"
		}
		if err := c.inner.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{Region: location}); err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
	}
	return nil
}

func (c *Client) Put(ctx context.Context, obj objectstorage.Object, reader io.Reader) error {
	opts := minio.PutObjectOptions{
		ContentType: obj.ContentType,
	}

	// Set custom metadata for retention tracking
	userMeta := map[string]string{
		"x-amz-meta-source-item-id": obj.Metadata.SourceItemID,
		"x-amz-meta-source-id":     obj.Metadata.SourceID,
		"x-amz-meta-user-id":       obj.Metadata.UserID,
		"x-amz-meta-platform":      obj.Metadata.Platform,
		"x-amz-meta-retention":     string(obj.Metadata.Retention),
		"x-amz-meta-original-url":  obj.Metadata.OriginalURL,
	}
	if obj.Metadata.ExpiresAt != nil {
		userMeta["x-amz-meta-expires-at"] = obj.Metadata.ExpiresAt.Format(time.RFC3339)
	}
	opts.UserMetadata = userMeta

	_, err := c.inner.PutObject(ctx, c.bucket, obj.Key, reader, obj.Size, opts)
	if err != nil {
		return fmt.Errorf("put object %s: %w", obj.Key, err)
	}
	return nil
}

func (c *Client) Get(ctx context.Context, key string) (objectstorage.Object, io.ReadCloser, error) {
	object, err := c.inner.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return objectstorage.Object{}, nil, fmt.Errorf("get object %s: %w", key, err)
	}

	info, err := object.Stat()
	if err != nil {
		return objectstorage.Object{}, nil, fmt.Errorf("stat object %s: %w", key, err)
	}

	obj := statToObject(key, info)
	return obj, object, nil
}

func (c *Client) Delete(ctx context.Context, key string) error {
	if err := c.inner.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}

func (c *Client) Head(ctx context.Context, key string) (objectstorage.Object, error) {
	info, err := c.inner.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return objectstorage.Object{}, fmt.Errorf("head object %s: %w", key, err)
	}
	return statToObject(key, info), nil
}

func (c *Client) ListExpired(ctx context.Context, bucket string, before time.Time) ([]objectstorage.Object, error) {
	if bucket == "" {
		bucket = c.bucket
	}

	var expired []objectstorage.Object
	for object := range c.inner.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if object.Err != nil {
			return nil, fmt.Errorf("list objects: %w", object.Err)
		}

		info, err := c.inner.StatObject(ctx, bucket, object.Key, minio.StatObjectOptions{})
		if err != nil {
			continue
		}

		obj := statToObject(object.Key, info)
		if obj.Metadata.ExpiresAt != nil && obj.Metadata.ExpiresAt.Before(before) {
			expired = append(expired, obj)
		}
	}
	return expired, nil
}

func (c *Client) ListByPrefix(ctx context.Context, prefix string) ([]objectstorage.Object, error) {
	var matched []objectstorage.Object
	for object := range c.inner.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if object.Err != nil {
			return nil, fmt.Errorf("list objects by prefix: %w", object.Err)
		}

		info, err := c.inner.StatObject(ctx, c.bucket, object.Key, minio.StatObjectOptions{})
		if err != nil {
			continue
		}

		matched = append(matched, statToObject(object.Key, info))
	}
	return matched, nil
}

func (c *Client) PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	url, err := c.inner.PresignedGetObject(ctx, c.bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presigned get url for %s: %w", key, err)
	}
	return url.String(), nil
}

// SnapshotContent builds a snapshot payload from text content.
func SnapshotContent(title, snippet, originalURL string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Title: %s\n\n", title)
	fmt.Fprintf(&buf, "URL: %s\n\n", originalURL)
	fmt.Fprintf(&buf, "Content:\n%s\n", snippet)
	return buf.Bytes()
}

func statToObject(key string, info minio.ObjectInfo) objectstorage.Object {
	meta := objectstorage.Metadata{
		SourceItemID: info.UserMetadata["x-amz-meta-source-item-id"],
		SourceID:     info.UserMetadata["x-amz-meta-source-id"],
		UserID:       info.UserMetadata["x-amz-meta-user-id"],
		Platform:     info.UserMetadata["x-amz-meta-platform"],
		Retention:    objectstorage.RetentionPolicy(info.UserMetadata["x-amz-meta-retention"]),
		OriginalURL:  info.UserMetadata["x-amz-meta-original-url"],
	}

	if expiresAt := info.UserMetadata["x-amz-meta-expires-at"]; expiresAt != "" {
		if t, err := time.Parse(time.RFC3339, expiresAt); err == nil {
			meta.ExpiresAt = &t
		}
	}

	return objectstorage.Object{
		Key:         key,
		ContentType: info.ContentType,
		Size:        info.Size,
		ETag:        info.ETag,
		Metadata:    meta,
		CreatedAt:   info.LastModified,
	}
}
