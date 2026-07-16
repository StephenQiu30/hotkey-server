package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

const onnxManifestVersion = "v1"

type onnxManifest struct {
	Version         string   `json:"version"`
	ModelSHA256     string   `json:"model_sha256"`
	TokenizerSHA256 string   `json:"tokenizer_sha256"`
	ModelVersion    string   `json:"model_version"`
	Dimensions      int      `json:"dimensions"`
	MaxTokens       int      `json:"max_tokens"`
	InputNames      []string `json:"input_names"`
	OutputName      string   `json:"output_name"`
	Pooling         string   `json:"pooling"`
	Normalization   string   `json:"normalization"`
}

func loadONNXManifest(path string) (onnxManifest, error) {
	if err := requireRegularFile(path); err != nil {
		return onnxManifest{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return onnxManifest{}, err
	}
	var manifest onnxManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return onnxManifest{}, err
	}
	if !manifest.valid() {
		return onnxManifest{}, errors.New("invalid ONNX manifest contract")
	}
	return manifest, nil
}

func (manifest onnxManifest) valid() bool {
	return manifest.Version == onnxManifestVersion &&
		validSHA256(manifest.ModelSHA256) && validSHA256(manifest.TokenizerSHA256) &&
		strings.TrimSpace(manifest.ModelVersion) != "" &&
		manifest.Dimensions == intelligencedomain.EmbeddingDimensions &&
		manifest.MaxTokens > 0 && manifest.MaxTokens <= 8192 &&
		len(manifest.InputNames) == 3 &&
		manifest.InputNames[0] == "input_ids" && manifest.InputNames[1] == "attention_mask" && manifest.InputNames[2] == "token_type_ids" &&
		manifest.OutputName == "last_hidden_state" &&
		manifest.Pooling == "cls_l2" && manifest.Normalization == "nfc"
}

func verifyArtifactSHA256(path, expected string) error {
	if err := requireRegularFile(path); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), expected) {
		return errors.New("ONNX artifact checksum mismatch")
	}
	return nil
}

func requireRegularFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("ONNX artifact path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("ONNX artifact is not a regular file")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
