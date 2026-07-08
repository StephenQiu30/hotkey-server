package pkg_test

import (
	"math"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

func TestVector384ScanValue(t *testing.T) {
	var v pkg.Vector384
	// pgvector binary format: dim(4 bytes) + 384*4 bytes of float32
	data := make([]byte, 4+384*4)
	binaryWriteUint32(data[:4], 384)
	for i := range 384 {
		val := float32(i) / 384.0
		binaryWriteFloat32(data[4+i*4:], val)
	}
	if err := v.Scan(data); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	// Check first and last values
	expected := float32(0.0)
	if v[0] != expected {
		t.Errorf("expected v[0] = %f, got %f", expected, v[0])
	}
	if v.Dim() != 384 {
		t.Errorf("expected Dim() = 384, got %d", v.Dim())
	}

	// Round-trip through Value()
	val, err := v.Value()
	if err != nil {
		t.Fatalf("Value failed: %v", err)
	}
	if val == nil {
		t.Fatal("Value returned nil")
	}
	var v2 pkg.Vector384
	if err := v2.Scan(val); err != nil {
		t.Fatalf("Scan of Value() result failed: %v", err)
	}
	for i := range 384 {
		if v[i] != v2[i] {
			t.Errorf("round-trip mismatch at [%d]: %f vs %f", i, v[i], v2[i])
			break
		}
	}
}

func TestVector384ScanNil(t *testing.T) {
	var v pkg.Vector384
	if err := v.Scan(nil); err != nil {
		t.Fatalf("Scan nil failed: %v", err)
	}
}

func TestVector384ScanShortData(t *testing.T) {
	var v pkg.Vector384
	if err := v.Scan([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestVector384ScanWrongDim(t *testing.T) {
	var v pkg.Vector384
	data := make([]byte, 4+128*4)
	binaryWriteUint32(data[:4], 128)
	if err := v.Scan(data); err == nil {
		t.Fatal("expected error for wrong dimension")
	}
}

func TestVector384ScanString(t *testing.T) {
	var v pkg.Vector384
	data := make([]byte, 4+384*4)
	binaryWriteUint32(data[:4], 384)
	str := string(data)
	if err := v.Scan(str); err != nil {
		t.Fatalf("Scan string failed: %v", err)
	}
}

// helpers
func binaryWriteUint32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func binaryWriteFloat32(b []byte, v float32) {
	u := math.Float32bits(v)
	b[0] = byte(u)
	b[1] = byte(u >> 8)
	b[2] = byte(u >> 16)
	b[3] = byte(u >> 24)
}
