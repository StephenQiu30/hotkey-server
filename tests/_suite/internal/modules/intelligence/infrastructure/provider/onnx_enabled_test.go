//go:build onnx && cgo

package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
)

func TestONNXProviderRejectsMissingRuntime(t *testing.T) {
	provider, err := NewONNXProvider(config.AIConfig{})
	if provider != nil {
		t.Fatalf("NewONNXProvider() provider = %#v, want nil", provider)
	}
	assertCode(t, err, intelligencedomain.CodeAIModelUnavailable)
}

func TestONNXProviderRejectsMissingModel(t *testing.T) {
	ai := onnxArtifactPaths(t)
	ai.ONNXModelPath = ""
	assertONNXUnavailable(t, ai)
}

func TestONNXProviderRejectsMissingTokenizer(t *testing.T) {
	ai := onnxArtifactPaths(t)
	ai.ONNXTokenizerPath = ""
	assertONNXUnavailable(t, ai)
}

func TestONNXProviderRejectsMissingManifest(t *testing.T) {
	ai := onnxArtifactPaths(t)
	ai.ONNXManifestPath = ""
	assertONNXUnavailable(t, ai)
}

func TestONNXProviderRejectsManifestContract(t *testing.T) {
	ai := onnxArtifactPaths(t)
	if err := os.WriteFile(ai.ONNXManifestPath, []byte(`{"version":"v1"}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	assertONNXUnavailable(t, ai)
}

func TestONNXProviderRejectsWrongArtifactSHA(t *testing.T) {
	ai := onnxArtifactPaths(t)
	modelSHA := sha256.Sum256([]byte("fixture"))
	if err := os.WriteFile(ai.ONNXManifestPath, []byte(`{"version":"v1","model_sha256":"`+hex.EncodeToString(modelSHA[:])+`","tokenizer_sha256":"`+strings.Repeat("0", 64)+`","model_version":"bge-m3-v1","dimensions":1024,"max_tokens":8192,"input_names":["input_ids","attention_mask","token_type_ids"],"output_name":"last_hidden_state","pooling":"cls_l2","normalization":"nfc"}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	assertONNXUnavailable(t, ai)
}

func assertONNXUnavailable(t *testing.T, ai config.AIConfig) {
	t.Helper()
	provider, err := NewONNXProvider(ai)
	if provider != nil {
		t.Fatalf("NewONNXProvider() provider = %#v, want nil", provider)
	}
	assertCode(t, err, intelligencedomain.CodeAIModelUnavailable)
}

func onnxArtifactPaths(t *testing.T) config.AIConfig {
	t.Helper()
	directory := t.TempDir()
	paths := config.AIConfig{
		ONNXRuntimeLibrary: filepath.Join(directory, "runtime.dylib"),
		ONNXModelPath:      filepath.Join(directory, "model.onnx"),
		ONNXTokenizerPath:  filepath.Join(directory, "tokenizer.json"),
		ONNXManifestPath:   filepath.Join(directory, "manifest.json"),
	}
	for _, path := range []string{paths.ONNXRuntimeLibrary, paths.ONNXModelPath, paths.ONNXTokenizerPath} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("write artifact %s: %v", path, err)
		}
	}
	return paths
}
