package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadONNXManifestAcceptsOnlyFixedEmbeddingContract(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "manifest.json")
	valid := `{"version":"v1","model_sha256":"` + strings.Repeat("a", 64) + `","tokenizer_sha256":"` + strings.Repeat("b", 64) + `","model_version":"bge-m3-v1","dimensions":1024,"max_tokens":8192,"input_names":["input_ids","attention_mask","token_type_ids"],"output_name":"last_hidden_state","pooling":"cls_l2","normalization":"nfc"}`
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	manifest, err := loadONNXManifest(path)
	if err != nil {
		t.Fatalf("loadONNXManifest() error = %v", err)
	}
	if manifest.ModelVersion != "bge-m3-v1" || manifest.Dimensions != 1024 || manifest.MaxTokens != 8192 {
		t.Fatalf("loadONNXManifest() = %#v, want fixed embedding contract", manifest)
	}

	for _, mutation := range []string{
		strings.Replace(valid, `"pooling":"cls_l2"`, `"pooling":"mean"`, 1),
		strings.Replace(valid, `"normalization":"nfc"`, `"normalization":"none"`, 1),
		strings.Replace(valid, `"dimensions":1024`, `"dimensions":768`, 1),
		strings.Replace(valid, `"input_ids","attention_mask","token_type_ids"`, `"input_ids","attention_mask","position_ids"`, 1),
		strings.Replace(valid, `"output_name":"last_hidden_state"`, `"output_name":"pooled_output"`, 1),
		strings.Replace(valid, `"model_sha256":"`+strings.Repeat("a", 64)+`"`, `"model_sha256":"invalid"`, 1),
	} {
		if err := os.WriteFile(path, []byte(mutation), 0o600); err != nil {
			t.Fatalf("write invalid manifest: %v", err)
		}
		if _, err := loadONNXManifest(path); err == nil {
			t.Fatalf("loadONNXManifest() error = nil for invalid manifest %s", mutation)
		}
	}
}

func TestVerifyArtifactSHA256RejectsChangedBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, []byte("verified artifact"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := verifyArtifactSHA256(path, "2127de9293abf1503418b9f78b3d530cdd2263417064815ee46b7ecdf1215ddc"); err != nil {
		t.Fatalf("verifyArtifactSHA256() error = %v", err)
	}
	if err := verifyArtifactSHA256(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("verifyArtifactSHA256() error = nil, want checksum rejection")
	}
}
