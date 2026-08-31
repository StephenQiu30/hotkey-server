package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	miniosdk "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const maximumReconciliationAssetBytes int64 = 4 << 20
const maximumReconciliationListedAssets = 100_000

type evidenceLineageAssetInspectionRecord struct {
	Exists    bool
	SHA256    string
	SizeBytes int64
}

type evidenceLineageStoredAssetRecord struct {
	Locator    string
	ModifiedAt time.Time
}

type rawEvidenceAssetInspector interface {
	InspectRawEvidenceObject(context.Context, string, int64) (evidenceLineageAssetInspectionRecord, error)
	ListRawEvidenceObjects(context.Context, int) ([]evidenceLineageStoredAssetRecord, error)
}

type vaultProjectionAssetInspector interface {
	InspectVaultProjection(context.Context, string, int64) (evidenceLineageAssetInspectionRecord, error)
	ListVaultProjections(context.Context, int) ([]evidenceLineageStoredAssetRecord, error)
}

type rawEvidenceMinIOInspector struct {
	client *miniosdk.Client
	bucket string
}

type vaultProjectionFileInspector struct {
	root string
}

func NewEvidenceLineageMaintenanceRepositoryWithStorage(runtime *database.Runtime, cfg config.Config) (*EvidenceLineageMaintenanceRepository, error) {
	rawInspector, err := newRawEvidenceMinIOInspector(cfg.MinIO)
	if err != nil {
		return nil, err
	}
	vaultInspector, err := newVaultProjectionFileInspector(cfg.VaultPath)
	if err != nil {
		return nil, err
	}
	return newEvidenceLineageStorageRepository(runtime, rawInspector, vaultInspector)
}

func NewEvidenceLineageMaintenanceRepositoryWithMinIO(runtime *database.Runtime, cfg config.MinIOConfig) (*EvidenceLineageMaintenanceRepository, error) {
	inspector, err := newRawEvidenceMinIOInspector(cfg)
	if err != nil {
		return nil, err
	}
	return newEvidenceLineageStorageRepository(runtime, inspector, nil)
}

func NewEvidenceLineageMaintenanceRepositoryWithVault(runtime *database.Runtime, vaultPath string) (*EvidenceLineageMaintenanceRepository, error) {
	inspector, err := newVaultProjectionFileInspector(vaultPath)
	if err != nil {
		return nil, err
	}
	return newEvidenceLineageStorageRepository(runtime, nil, inspector)
}

func newEvidenceLineageStorageRepository(runtime *database.Runtime, raw rawEvidenceAssetInspector, vault vaultProjectionAssetInspector) (*EvidenceLineageMaintenanceRepository, error) {
	if runtime == nil || runtime.SQL == nil || runtime.Pool == nil {
		return nil, errors.New("database runtime is required")
	}
	return newEvidenceLineageMaintenanceRepository(runtime, raw, vault), nil
}

func newRawEvidenceMinIOInspector(cfg config.MinIOConfig) (*rawEvidenceMinIOInspector, error) {
	if err := cfg.ValidateRuntime(); err != nil {
		return nil, fmt.Errorf("evidence reconciliation requires MinIO: %w", err)
	}
	client, err := miniosdk.New(cfg.Endpoint, &miniosdk.Options{
		Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: cfg.UseSSL,
		Region: "us-east-1", BucketLookup: miniosdk.BucketLookupPath, MaxRetries: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("create evidence reconciliation MinIO client: %w", err)
	}
	return &rawEvidenceMinIOInspector{client: client, bucket: cfg.Bucket}, nil
}

func newVaultProjectionFileInspector(vaultPath string) (*vaultProjectionFileInspector, error) {
	root, err := filepath.Abs(filepath.Clean(vaultPath))
	if err != nil || strings.TrimSpace(vaultPath) == "" {
		return nil, errors.New("evidence reconciliation Vault root is invalid")
	}
	return &vaultProjectionFileInspector{root: root}, nil
}

func (inspector *rawEvidenceMinIOInspector) InspectRawEvidenceObject(ctx context.Context, objectKey string, maximumBytes int64) (evidenceLineageAssetInspectionRecord, error) {
	if inspector == nil || inspector.client == nil || inspector.bucket == "" || !validRawEvidenceObjectKey(objectKey) || maximumBytes <= 0 || maximumBytes > maximumReconciliationAssetBytes {
		return evidenceLineageAssetInspectionRecord{}, errors.New("raw evidence inspection input is invalid")
	}
	info, err := inspector.client.StatObject(ctx, inspector.bucket, objectKey, miniosdk.StatObjectOptions{})
	if err != nil {
		if missingMinIOObject(err) {
			return evidenceLineageAssetInspectionRecord{Exists: false}, nil
		}
		return evidenceLineageAssetInspectionRecord{}, fmt.Errorf("head raw evidence object: %w", err)
	}
	if info.Size < 0 || info.Size > maximumBytes {
		return evidenceLineageAssetInspectionRecord{}, errors.New("raw evidence object exceeds reconciliation limit")
	}
	object, err := inspector.client.GetObject(ctx, inspector.bucket, objectKey, miniosdk.GetObjectOptions{})
	if err != nil {
		return evidenceLineageAssetInspectionRecord{}, fmt.Errorf("get raw evidence object: %w", err)
	}
	defer func() { _ = object.Close() }()
	payload, err := io.ReadAll(io.LimitReader(object, maximumBytes+1))
	if err != nil || int64(len(payload)) > maximumBytes || int64(len(payload)) != info.Size {
		return evidenceLineageAssetInspectionRecord{}, errors.New("raw evidence object bytes changed during reconciliation")
	}
	digest := sha256.Sum256(payload)
	return evidenceLineageAssetInspectionRecord{Exists: true, SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(payload))}, nil
}

func (inspector *rawEvidenceMinIOInspector) ListRawEvidenceObjects(ctx context.Context, maximumAssets int) ([]evidenceLineageStoredAssetRecord, error) {
	if inspector == nil || inspector.client == nil || inspector.bucket == "" || maximumAssets < 1 || maximumAssets > maximumReconciliationListedAssets {
		return nil, errors.New("raw evidence listing input is invalid")
	}
	assets := make([]evidenceLineageStoredAssetRecord, 0)
	for object := range inspector.client.ListObjects(ctx, inspector.bucket, miniosdk.ListObjectsOptions{Prefix: "source-raw/v1/", Recursive: true}) {
		if object.Err != nil {
			return nil, fmt.Errorf("list raw evidence objects: %w", object.Err)
		}
		if len(assets) >= maximumAssets {
			return nil, errors.New("raw evidence object listing exceeds reconciliation limit")
		}
		assets = append(assets, evidenceLineageStoredAssetRecord{Locator: object.Key, ModifiedAt: object.LastModified.UTC()})
	}
	sort.Slice(assets, func(left, right int) bool { return assets[left].Locator < assets[right].Locator })
	return assets, nil
}

func (inspector *vaultProjectionFileInspector) InspectVaultProjection(ctx context.Context, relativePath string, maximumBytes int64) (evidenceLineageAssetInspectionRecord, error) {
	if inspector == nil || inspector.root == "" || !validVaultProjectionRelativePath(relativePath) || maximumBytes <= 0 || maximumBytes > maximumReconciliationAssetBytes {
		return evidenceLineageAssetInspectionRecord{}, errors.New("vault projection inspection input is invalid")
	}
	if err := ctx.Err(); err != nil {
		return evidenceLineageAssetInspectionRecord{}, err
	}
	root, err := os.OpenRoot(inspector.root)
	if errors.Is(err, os.ErrNotExist) {
		return evidenceLineageAssetInspectionRecord{Exists: false}, nil
	}
	if err != nil {
		return evidenceLineageAssetInspectionRecord{}, err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Lstat(relativePath)
	if errors.Is(err, os.ErrNotExist) {
		return evidenceLineageAssetInspectionRecord{Exists: false}, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maximumBytes {
		return evidenceLineageAssetInspectionRecord{}, errors.New("vault projection path is not a bounded regular file")
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return evidenceLineageAssetInspectionRecord{}, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return evidenceLineageAssetInspectionRecord{}, errors.New("vault projection changed during reconciliation")
	}
	payload, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil || int64(len(payload)) > maximumBytes || int64(len(payload)) != info.Size() {
		return evidenceLineageAssetInspectionRecord{}, errors.New("vault projection bytes changed during reconciliation")
	}
	digest := sha256.Sum256(payload)
	return evidenceLineageAssetInspectionRecord{Exists: true, SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(payload))}, nil
}

func (inspector *vaultProjectionFileInspector) ListVaultProjections(ctx context.Context, maximumAssets int) ([]evidenceLineageStoredAssetRecord, error) {
	if inspector == nil || inspector.root == "" || maximumAssets < 1 || maximumAssets > maximumReconciliationListedAssets {
		return nil, errors.New("vault projection listing input is invalid")
	}
	assets := make([]evidenceLineageStoredAssetRecord, 0)
	documentsRoot := filepath.Join(inspector.root, "documents")
	err := filepath.WalkDir(documentsRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) && path == documentsRoot {
				return filepath.SkipDir
			}
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		if len(assets) >= maximumAssets {
			return errors.New("vault projection listing exceeds reconciliation limit")
		}
		relativePath, err := filepath.Rel(inspector.root, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if !validVaultProjectionRelativePath(relativePath) {
			return errors.New("vault projection listing encountered an invalid path")
		}
		assets = append(assets, evidenceLineageStoredAssetRecord{Locator: relativePath, ModifiedAt: info.ModTime().UTC()})
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return []evidenceLineageStoredAssetRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list Vault projections: %w", err)
	}
	sort.Slice(assets, func(left, right int) bool { return assets[left].Locator < assets[right].Locator })
	return assets, nil
}

func validRawEvidenceObjectKey(value string) bool {
	if !strings.HasPrefix(value, "source-raw/v1/") || filepath.IsAbs(value) || strings.Contains(value, "\\") || filepath.ToSlash(filepath.Clean(value)) != value {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) != 5 || parts[0] != "source-raw" || parts[1] != "v1" || parts[2] == "" || len(parts[3]) != 2 || !strings.HasSuffix(parts[4], ".raw") {
		return false
	}
	digest := strings.TrimSuffix(parts[4], ".raw")
	return len(digest) == 64 && parts[3] == digest[:2] && strings.Trim(digest, "0123456789abcdef") == ""
}

func validVaultProjectionRelativePath(value string) bool {
	return value != "" && !filepath.IsAbs(value) && !strings.Contains(value, "\\") && filepath.ToSlash(filepath.Clean(value)) == value && strings.HasPrefix(value, "documents/")
}

func missingMinIOObject(err error) bool {
	var response miniosdk.ErrorResponse
	return errors.As(err, &response) && (response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NoSuchBucket")
}
