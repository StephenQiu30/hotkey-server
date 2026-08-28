package sourcenet

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type compressionBombFixture struct {
	Cases []compressionBombCase `json:"cases"`
}

type compressionBombCase struct {
	Name                string `json:"name"`
	Kind                string `json:"kind"`
	ExpandedBytes       int    `json:"expanded_bytes"`
	Members             int    `json:"members"`
	Depth               int    `json:"depth"`
	SleepMilliseconds   int    `json:"sleep_milliseconds"`
	MaxEncodedBytes     int64  `json:"max_encoded_bytes"`
	MaxDecodedBytes     int64  `json:"max_decoded_bytes"`
	MaxRatio            int64  `json:"max_ratio"`
	MaxMembers          int    `json:"max_members"`
	MaxDepth            int    `json:"max_depth"`
	MaxWallMilliseconds int    `json:"max_wall_milliseconds"`
	Want                string `json:"want"`
}

func TestCompressionBombFixtureStopsBeforeReturningAnyDecodedPayload(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	fixture := readCompressionBombFixture(t)
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			body, encoding := compressionBombReader(t, test)
			limits := CompressionLimits{
				MaxEncodedBytes: test.MaxEncodedBytes, MaxDecodedBytes: test.MaxDecodedBytes,
				MaxRatio: test.MaxRatio, MaxMembers: test.MaxMembers, MaxDepth: test.MaxDepth,
				MaxWallClock: time.Duration(test.MaxWallMilliseconds) * time.Millisecond,
			}
			for attempt := 0; attempt < 2; attempt++ {
				payload, err := ReadBoundedResponse(context.Background(), body(), encoding, limits)
				if len(payload) != 0 {
					t.Fatalf("attempt %d returned %d decoded bytes", attempt+1, len(payload))
				}
				if !errors.Is(err, compressionFixtureError(test.Want)) {
					t.Fatalf("attempt %d error = %v, want %s", attempt+1, err, test.Want)
				}
			}
		})
	}
	entries, err := os.ReadDir(scratch)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("compression guard left temporary artifacts: %#v", entries)
	}
}

func TestCompressionGuardAllowsOneBoundedGZIPMember(t *testing.T) {
	want := []byte(`<?xml version="1.0"?><rss><channel><title>safe</title></channel></rss>`)
	encoded := gzipBytes(t, want)
	got, err := ReadBoundedResponse(context.Background(), bytes.NewReader(encoded), "gzip", CompressionLimits{
		MaxEncodedBytes: 4096, MaxDecodedBytes: 4096, MaxRatio: 100,
		MaxMembers: 1, MaxDepth: 1, MaxWallClock: 100 * time.Millisecond,
	})
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("bounded gzip = %q / %v", got, err)
	}
}

func TestDefaultCompressionLimitsAreVersionedAndFrozen(t *testing.T) {
	limits := DefaultCompressionLimits(4 << 20)
	if CompressionProfileVersion != "source-response-compression-v1" || limits.MaxEncodedBytes != 4<<20 ||
		limits.MaxDecodedBytes != 4<<20 || limits.MaxRatio != 100 || limits.MaxMembers != 1 || limits.MaxDepth != 1 ||
		limits.MaxWallClock != 2*time.Second || limits.Validate() != nil {
		t.Fatalf("compression profile = %q/%#v", CompressionProfileVersion, limits)
	}
}

func readCompressionBombFixture(t *testing.T) compressionBombFixture {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join("testdata", "security", "compression_bombs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture compressionBombFixture
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("compression bomb fixture must not be empty")
	}
	return fixture
}

func compressionBombReader(t *testing.T, test compressionBombCase) (func() io.Reader, string) {
	t.Helper()
	plain := bytes.Repeat([]byte("A"), test.ExpandedBytes)
	switch test.Kind {
	case "gzip":
		encoded := gzipBytes(t, plain)
		return func() io.Reader { return bytes.NewReader(encoded) }, "gzip"
	case "gzip_multistream":
		var encoded bytes.Buffer
		for member := 0; member < test.Members; member++ {
			writer := gzip.NewWriter(&encoded)
			if _, err := writer.Write(plain); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
		}
		payload := encoded.Bytes()
		return func() io.Reader { return bytes.NewReader(payload) }, "gzip"
	case "nested_gzip":
		payload := plain
		for depth := 0; depth < test.Depth; depth++ {
			payload = gzipBytes(t, payload)
		}
		return func() io.Reader { return bytes.NewReader(payload) }, "gzip"
	case "forged_gzip_length":
		payload := gzipBytes(t, plain)
		payload[len(payload)-4] ^= 0xff
		return func() io.Reader { return bytes.NewReader(payload) }, "gzip"
	case "zip":
		var encoded bytes.Buffer
		writer := zip.NewWriter(&encoded)
		for member := 0; member < test.Members; member++ {
			file, err := writer.Create(fmt.Sprintf("member-%02d.txt", member))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write(plain); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		payload := encoded.Bytes()
		return func() io.Reader { return bytes.NewReader(payload) }, "identity"
	case "slow_identity":
		return func() io.Reader {
			return &slowCompressionReader{payload: plain, delay: time.Duration(test.SleepMilliseconds) * time.Millisecond}
		}, "identity"
	default:
		t.Fatalf("unknown compression fixture kind %q", test.Kind)
		return nil, ""
	}
}

func gzipBytes(t *testing.T, plain []byte) []byte {
	t.Helper()
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	if _, err := writer.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), encoded.Bytes()...)
}

func compressionFixtureError(name string) error {
	switch name {
	case "encoded_bytes":
		return ErrEncodedBytesExceeded
	case "decoded_bytes":
		return ErrDecodedBytesExceeded
	case "ratio":
		return ErrCompressionRatioExceeded
	case "members":
		return ErrCompressionMembersExceeded
	case "depth":
		return ErrCompressionDepthExceeded
	case "invalid":
		return ErrInvalidCompressedResponse
	case "archive":
		return ErrArchiveResponseNotPermitted
	case "wall_clock":
		return ErrDecompressionWallClockExceeded
	default:
		return errors.New("unknown compression fixture error " + strings.TrimSpace(name))
	}
}

type slowCompressionReader struct {
	payload []byte
	delay   time.Duration
	done    bool
}

func (reader *slowCompressionReader) Read(target []byte) (int, error) {
	if reader.done {
		return 0, io.EOF
	}
	time.Sleep(reader.delay)
	reader.done = true
	return copy(target, reader.payload), nil
}
