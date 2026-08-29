//go:build integration

package minio_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const minioAdminImage = "minio/mc@sha256:a7fe349ef4bd8521fb8497f55c6042871b2ae640607cf99d9bede5e9bdf11727"

func TestMinIOCredentialRotationPrechecksRollsBackAndRevokesScopedOldUser(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ENDPOINT"))
	adminEndpoint := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ADMIN_ENDPOINT"))
	network := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_DOCKER_NETWORK"))
	adminAccessKey := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_ACCESS_KEY"))
	adminSecretKey := strings.TrimSpace(os.Getenv("HOTKEY_TEST_MINIO_SECRET_KEY"))
	if endpoint == "" || adminEndpoint == "" || network == "" || adminAccessKey == "" || adminSecretKey == "" {
		t.Fatal("isolated MinIO endpoint, admin endpoint, Docker network and administrator credentials are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	root := newRotationMinIOClient(t, endpoint, adminAccessKey, adminSecretKey)
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	if len(suffix) > 10 {
		suffix = suffix[len(suffix)-10:]
	}
	bucket := "hotkey-rotation-" + suffix
	policy := "hotkey-rotation-" + suffix
	oldAccessKey := "hkold" + suffix
	newAccessKey := "hknew" + suffix
	oldSecretKey := "synthetic-minio-old-credential-0123456789"
	newSecretKey := "synthetic-minio-new-credential-0123456789"
	invalidSecretKey := "synthetic-minio-invalid-credential-012345"
	if err := root.MakeBucket(ctx, bucket, miniosdk.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		t.Fatalf("create isolated MinIO rotation bucket: %v", err)
	}
	policyDocument, err := json.Marshal(map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{"Effect": "Allow", "Action": []string{"s3:GetBucketLocation", "s3:ListBucket"}, "Resource": []string{"arn:aws:s3:::" + bucket}},
			{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject"}, "Resource": []string{"arn:aws:s3:::" + bucket + "/*"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	adminEnvironment := map[string]string{
		"ADMIN_ENDPOINT":   adminEndpoint,
		"ADMIN_ACCESS_KEY": adminAccessKey,
		"ADMIN_SECRET_KEY": adminSecretKey,
		"OLD_ACCESS_KEY":   oldAccessKey,
		"OLD_SECRET_KEY":   oldSecretKey,
		"NEW_ACCESS_KEY":   newAccessKey,
		"NEW_SECRET_KEY":   newSecretKey,
		"POLICY_NAME":      policy,
		"POLICY_JSON":      string(policyDocument),
	}
	runMinIOAdmin(t, ctx, network, adminEnvironment, `
mc alias set fixture "$ADMIN_ENDPOINT" "$ADMIN_ACCESS_KEY" "$ADMIN_SECRET_KEY" >/dev/null
printf '%s' "$POLICY_JSON" >/tmp/policy.json
mc admin policy create fixture "$POLICY_NAME" /tmp/policy.json >/dev/null
mc admin user add fixture "$OLD_ACCESS_KEY" "$OLD_SECRET_KEY" >/dev/null
mc admin user add fixture "$NEW_ACCESS_KEY" "$NEW_SECRET_KEY" >/dev/null
mc admin policy attach fixture "$POLICY_NAME" --user "$OLD_ACCESS_KEY" >/dev/null
mc admin policy attach fixture "$POLICY_NAME" --user "$NEW_ACCESS_KEY" >/dev/null
`)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		cleanupEnvironment := map[string]string{
			"ADMIN_ENDPOINT":   adminEndpoint,
			"ADMIN_ACCESS_KEY": adminAccessKey,
			"ADMIN_SECRET_KEY": adminSecretKey,
			"OLD_ACCESS_KEY":   oldAccessKey,
			"NEW_ACCESS_KEY":   newAccessKey,
			"POLICY_NAME":      policy,
		}
		_ = runMinIOAdminCommand(cleanupCtx, network, cleanupEnvironment, `
mc alias set fixture "$ADMIN_ENDPOINT" "$ADMIN_ACCESS_KEY" "$ADMIN_SECRET_KEY" >/dev/null
mc admin user remove fixture "$OLD_ACCESS_KEY" >/dev/null 2>&1 || true
mc admin user remove fixture "$NEW_ACCESS_KEY" >/dev/null 2>&1 || true
mc admin policy remove fixture "$POLICY_NAME" >/dev/null 2>&1 || true
`)
		objects := root.ListObjects(cleanupCtx, bucket, miniosdk.ListObjectsOptions{Recursive: true})
		for object := range objects {
			if object.Err == nil {
				_ = root.RemoveObject(cleanupCtx, bucket, object.Key, miniosdk.RemoveObjectOptions{})
			}
		}
		_ = root.RemoveBucket(cleanupCtx, bucket)
	})

	oldClient := newRotationMinIOClient(t, endpoint, oldAccessKey, oldSecretKey)
	newClient := newRotationMinIOClient(t, endpoint, newAccessKey, newSecretKey)
	assertRotationMinIOWriteRead(t, ctx, oldClient, bucket, "old-baseline", "preserved-old-object")
	assertRotationMinIOWriteRead(t, ctx, newClient, bucket, "new-preflight", "preflight-new-object")
	invalidClient := newRotationMinIOClient(t, endpoint, newAccessKey, invalidSecretKey)
	if _, err := invalidClient.StatObject(ctx, bucket, "old-baseline", miniosdk.StatObjectOptions{}); err == nil {
		t.Fatal("invalid candidate MinIO credential unexpectedly passed preflight")
	} else if strings.Contains(err.Error(), invalidSecretKey) {
		t.Fatal("MinIO preflight error exposed candidate credential plaintext")
	}
	assertRotationMinIORead(t, ctx, oldClient, bucket, "old-baseline", "preserved-old-object")
	assertRotationMinIORead(t, ctx, newClient, bucket, "old-baseline", "preserved-old-object")

	runMinIOAdmin(t, ctx, network, adminEnvironment, `
mc alias set fixture "$ADMIN_ENDPOINT" "$ADMIN_ACCESS_KEY" "$ADMIN_SECRET_KEY" >/dev/null
mc admin user disable fixture "$OLD_ACCESS_KEY" >/dev/null
`)
	revokedClient := newRotationMinIOClient(t, endpoint, oldAccessKey, oldSecretKey)
	if _, err := revokedClient.StatObject(ctx, bucket, "old-baseline", miniosdk.StatObjectOptions{}); err == nil {
		t.Fatal("revoked old MinIO credential remained usable")
	} else if strings.Contains(err.Error(), oldSecretKey) {
		t.Fatal("MinIO revocation error exposed old credential plaintext")
	}
	assertRotationMinIORead(t, ctx, newClient, bucket, "old-baseline", "preserved-old-object")
}

func newRotationMinIOClient(t *testing.T, endpoint, accessKey, secretKey string) *miniosdk.Client {
	t.Helper()
	client, err := miniosdk.New(endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Region: "us-east-1",
		BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("create isolated MinIO credential client: %v", err)
	}
	return client
}

func assertRotationMinIOWriteRead(t *testing.T, ctx context.Context, client *miniosdk.Client, bucket, key, body string) {
	t.Helper()
	if _, err := client.PutObject(ctx, bucket, key, strings.NewReader(body), int64(len(body)), miniosdk.PutObjectOptions{ContentType: "text/plain"}); err != nil {
		t.Fatalf("write isolated MinIO credential probe: %v", err)
	}
	assertRotationMinIORead(t, ctx, client, bucket, key, body)
}

func assertRotationMinIORead(t *testing.T, ctx context.Context, client *miniosdk.Client, bucket, key, expected string) {
	t.Helper()
	object, err := client.GetObject(ctx, bucket, key, miniosdk.GetObjectOptions{})
	if err != nil {
		t.Fatalf("open isolated MinIO credential probe: %v", err)
	}
	contents, readErr := io.ReadAll(object)
	closeErr := object.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read isolated MinIO credential probe: read=%v close=%v", readErr, closeErr)
	}
	if string(contents) != expected {
		t.Fatal("isolated MinIO credential probe content changed during rotation")
	}
}

func runMinIOAdmin(t *testing.T, ctx context.Context, network string, environment map[string]string, script string) {
	t.Helper()
	if err := runMinIOAdminCommand(ctx, network, environment, script); err != nil {
		t.Fatal("isolated MinIO credential administration failed")
	}
}

func runMinIOAdminCommand(ctx context.Context, network string, environment map[string]string, script string) error {
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	arguments := []string{"run", "--rm", "--network", network}
	commandEnvironment := append([]string(nil), os.Environ()...)
	for _, name := range names {
		arguments = append(arguments, "--env", name)
		commandEnvironment = append(commandEnvironment, name+"="+environment[name])
	}
	arguments = append(arguments, "--entrypoint", "/bin/sh", minioAdminImage, "-eu", "-c", script)
	command := exec.CommandContext(ctx, "docker", arguments...)
	command.Env = commandEnvironment
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}
