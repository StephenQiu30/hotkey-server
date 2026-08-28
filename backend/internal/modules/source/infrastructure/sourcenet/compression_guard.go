package sourcenet

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const CompressionProfileVersion = "source-response-compression-v1"

var (
	ErrEncodedBytesExceeded             = errors.New("source response encoded byte limit exceeded")
	ErrDecodedBytesExceeded             = errors.New("source response decoded byte limit exceeded")
	ErrCompressionRatioExceeded         = errors.New("source response compression ratio exceeded")
	ErrCompressionMembersExceeded       = errors.New("source response compression member limit exceeded")
	ErrCompressionDepthExceeded         = errors.New("source response compression depth exceeded")
	ErrDecompressionWallClockExceeded   = errors.New("source response decompression wall-clock limit exceeded")
	ErrInvalidCompressedResponse        = errors.New("source response compression is invalid")
	ErrArchiveResponseNotPermitted      = errors.New("source archive response is not permitted")
	ErrUnsupportedResponseContentCoding = errors.New("source response content coding is not permitted")
	ErrResponseBodyRead                 = errors.New("source response body read failed")
)

// CompressionLimits bounds both the wire representation and the decoded
// payload before a connector can construct Evidence or write an object.
type CompressionLimits struct {
	MaxEncodedBytes int64
	MaxDecodedBytes int64
	MaxRatio        int64
	MaxMembers      int
	MaxDepth        int
	MaxWallClock    time.Duration
}

func (limits CompressionLimits) Validate() error {
	if limits.MaxEncodedBytes < 1 || limits.MaxEncodedBytes > 64<<20 ||
		limits.MaxDecodedBytes < 1 || limits.MaxDecodedBytes > 64<<20 ||
		limits.MaxRatio < 1 || limits.MaxRatio > 1000 || limits.MaxMembers < 1 || limits.MaxMembers > 16 ||
		limits.MaxDepth < 1 || limits.MaxDepth > 4 || limits.MaxWallClock <= 0 || limits.MaxWallClock > 30*time.Second {
		return fmt.Errorf("invalid source response compression limits")
	}
	return nil
}

// DefaultCompressionLimits permits one gzip member and rejects nested
// archives. The decoded byte ceiling remains the connector's cumulative
// response budget; compressed bytes can never exceed that same budget.
func DefaultCompressionLimits(maxDecodedBytes int64) CompressionLimits {
	return CompressionLimits{
		MaxEncodedBytes: maxDecodedBytes, MaxDecodedBytes: maxDecodedBytes,
		MaxRatio: 100, MaxMembers: 1, MaxDepth: 1, MaxWallClock: 2 * time.Second,
	}
}

// ReadBoundedResponse is the only decompression entry point for P0 source
// connectors. It keeps encoded and decoded bytes in bounded memory, never
// creates a temporary file, and returns no partial payload on rejection.
func ReadBoundedResponse(ctx context.Context, body io.Reader, contentEncoding string, limits CompressionLimits) ([]byte, error) {
	if body == nil || limits.Validate() != nil {
		return nil, ErrInvalidCompressedResponse
	}
	if ctx == nil {
		ctx = context.Background()
	}
	guardCtx, cancel := context.WithTimeout(ctx, limits.MaxWallClock)
	defer cancel()

	encoded, err := readBoundedEncoded(guardCtx, body, limits.MaxEncodedBytes)
	if err != nil {
		return nil, err
	}
	encoding := strings.ToLower(strings.TrimSpace(contentEncoding))
	switch encoding {
	case "", "identity":
		if archivePayload(encoded) {
			return nil, ErrArchiveResponseNotPermitted
		}
		if int64(len(encoded)) > limits.MaxDecodedBytes {
			return nil, ErrDecodedBytesExceeded
		}
		return encoded, nil
	case "gzip", "x-gzip":
		return decodeGZIP(guardCtx, encoded, limits, 1, int64(len(encoded)))
	default:
		return nil, ErrUnsupportedResponseContentCoding
	}
}

func readBoundedEncoded(ctx context.Context, source io.Reader, limit int64) ([]byte, error) {
	var result bytes.Buffer
	buffer := make([]byte, 32<<10)
	for {
		if err := compressionContextError(ctx); err != nil {
			return nil, err
		}
		read, err := source.Read(buffer)
		if contextErr := compressionContextError(ctx); contextErr != nil {
			return nil, contextErr
		}
		if read > 0 {
			if int64(result.Len()+read) > limit {
				return nil, ErrEncodedBytesExceeded
			}
			_, _ = result.Write(buffer[:read])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return result.Bytes(), nil
			}
			return nil, ErrResponseBodyRead
		}
		if read == 0 {
			return nil, ErrInvalidCompressedResponse
		}
	}
}

func decodeGZIP(ctx context.Context, encoded []byte, limits CompressionLimits, depth int, rootEncodedBytes int64) ([]byte, error) {
	if depth > limits.MaxDepth {
		return nil, ErrCompressionDepthExceeded
	}
	remaining := bytes.NewReader(encoded)
	var decoded bytes.Buffer
	members := 0
	buffer := make([]byte, 32<<10)
	for remaining.Len() > 0 {
		if members >= limits.MaxMembers {
			return nil, ErrCompressionMembersExceeded
		}
		reader, err := gzip.NewReader(remaining)
		if err != nil {
			return nil, ErrInvalidCompressedResponse
		}
		reader.Multistream(false)
		members++
		for {
			if err := compressionContextError(ctx); err != nil {
				_ = reader.Close()
				return nil, err
			}
			read, readErr := reader.Read(buffer)
			if contextErr := compressionContextError(ctx); contextErr != nil {
				_ = reader.Close()
				return nil, contextErr
			}
			if read > 0 {
				nextDecoded := int64(decoded.Len() + read)
				if nextDecoded > limits.MaxDecodedBytes {
					_ = reader.Close()
					return nil, ErrDecodedBytesExceeded
				}
				if nextDecoded > rootEncodedBytes*limits.MaxRatio {
					_ = reader.Close()
					return nil, ErrCompressionRatioExceeded
				}
				_, _ = decoded.Write(buffer[:read])
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					_ = reader.Close()
					return nil, ErrInvalidCompressedResponse
				}
				break
			}
			if read == 0 {
				_ = reader.Close()
				return nil, ErrInvalidCompressedResponse
			}
		}
		if err := reader.Close(); err != nil {
			return nil, ErrInvalidCompressedResponse
		}
	}
	payload := decoded.Bytes()
	if gzipPayload(payload) {
		if depth >= limits.MaxDepth {
			return nil, ErrCompressionDepthExceeded
		}
		return decodeGZIP(ctx, payload, limits, depth+1, rootEncodedBytes)
	}
	if archivePayload(payload) {
		return nil, ErrArchiveResponseNotPermitted
	}
	return append([]byte(nil), payload...), nil
}

func compressionContextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ErrDecompressionWallClockExceeded
		}
		return ctx.Err()
	default:
		return nil
	}
}

func gzipPayload(payload []byte) bool {
	return len(payload) >= 2 && payload[0] == 0x1f && payload[1] == 0x8b
}

func archivePayload(payload []byte) bool {
	if gzipPayload(payload) || bytes.HasPrefix(payload, []byte{'P', 'K', 0x03, 0x04}) ||
		bytes.HasPrefix(payload, []byte{'P', 'K', 0x05, 0x06}) || bytes.HasPrefix(payload, []byte{'P', 'K', 0x07, 0x08}) ||
		bytes.HasPrefix(payload, []byte{'B', 'Z', 'h'}) || bytes.HasPrefix(payload, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}) ||
		bytes.HasPrefix(payload, []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}) || bytes.HasPrefix(payload, []byte{'R', 'a', 'r', '!', 0x1a, 0x07}) ||
		bytes.HasPrefix(payload, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		return true
	}
	return len(payload) >= 262 && bytes.Equal(payload[257:262], []byte("ustar"))
}
