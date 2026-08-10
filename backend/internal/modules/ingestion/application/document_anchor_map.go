package application

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	CanonicalDocumentTextNormalizationVersion = "nfc-lf-collapse-space-v1"
	CanonicalDocumentAnchorMapProfileVersion  = "commonmark-gfm-visible-blocks-v1"
	maximumDocumentAnchorBlocks               = 20000
)

var markdownAnchorPattern = regexp.MustCompile(`^body-[0-9]{4,5}-[0-9a-f]{12}$`)

// MapDocumentTextCommand carries one bounded Markdown projection. The mapper
// returns canonical plaintext and offsets, but persistence DTOs never carry
// either body representation.
type MapDocumentTextCommand struct {
	Markdown string
}

// DocumentAnchorBlockDTO is an immutable offset-only mapping fact. All ranges
// are UTF-8 byte offsets and use left-closed/right-open semantics.
type DocumentAnchorBlockDTO struct {
	Ordinal                int
	PlaintextUTF8ByteStart int64
	PlaintextUTF8ByteEnd   int64
	MarkdownUTF8ByteStart  int64
	MarkdownUTF8ByteEnd    int64
	MarkdownAnchor         string
}

type MapDocumentTextResult struct {
	Plaintext               string
	NormalizationVersion    string
	AnchorMapProfileVersion string
	PlaintextSHA256         string
	MarkdownSHA256          string
	AnchorMapSHA256         string
	Blocks                  []DocumentAnchorBlockDTO
}

type DocumentTextAnchorMapper interface {
	MapDocumentText(context.Context, MapDocumentTextCommand) (MapDocumentTextResult, error)
}

func ValidateMapDocumentTextCommand(command MapDocumentTextCommand) error {
	if command.Markdown == "" || len(command.Markdown) > MaximumMarkdownProjectionBytes ||
		!utf8.ValidString(command.Markdown) || strings.ContainsRune(command.Markdown, '\r') ||
		norm.NFC.String(command.Markdown) != command.Markdown {
		return errors.New("document text mapping command is invalid")
	}
	return nil
}

func ValidateMapDocumentTextResult(command MapDocumentTextCommand, result MapDocumentTextResult) error {
	if err := ValidateMapDocumentTextCommand(command); err != nil {
		return err
	}
	if result.Plaintext == "" || len(result.Plaintext) > MaximumCanonicalSourceBodyBytes ||
		!utf8.ValidString(result.Plaintext) || strings.ContainsRune(result.Plaintext, '\r') ||
		norm.NFC.String(result.Plaintext) != result.Plaintext ||
		result.NormalizationVersion != CanonicalDocumentTextNormalizationVersion ||
		result.AnchorMapProfileVersion != CanonicalDocumentAnchorMapProfileVersion ||
		result.PlaintextSHA256 != documentTextSHA256(result.Plaintext) ||
		result.MarkdownSHA256 != documentTextSHA256(command.Markdown) ||
		len(result.Blocks) == 0 || len(result.Blocks) > maximumDocumentAnchorBlocks {
		return errors.New("document text mapping result is invalid")
	}
	anchors := make(map[string]struct{}, len(result.Blocks))
	var previousPlaintextEnd, previousMarkdownEnd int64
	for index, block := range result.Blocks {
		if block.Ordinal != index || !ValidUTF8DocumentTextRange(result.Plaintext, block.PlaintextUTF8ByteStart, block.PlaintextUTF8ByteEnd) ||
			!ValidUTF8DocumentTextRange(command.Markdown, block.MarkdownUTF8ByteStart, block.MarkdownUTF8ByteEnd) ||
			!markdownAnchorPattern.MatchString(block.MarkdownAnchor) {
			return errors.New("document anchor block is invalid")
		}
		if index == 0 {
			if block.PlaintextUTF8ByteStart != 0 {
				return errors.New("document anchor map does not start at plaintext byte zero")
			}
		} else {
			if block.PlaintextUTF8ByteStart <= previousPlaintextEnd || block.MarkdownUTF8ByteStart < previousMarkdownEnd ||
				result.Plaintext[previousPlaintextEnd:block.PlaintextUTF8ByteStart] != "\n\n" {
				return errors.New("document anchor blocks overlap or do not cover canonical plaintext")
			}
		}
		blockText := result.Plaintext[block.PlaintextUTF8ByteStart:block.PlaintextUTF8ByteEnd]
		if block.MarkdownAnchor != DocumentMarkdownAnchor(index, blockText) {
			return errors.New("document anchor does not match its canonical plaintext block")
		}
		if _, exists := anchors[block.MarkdownAnchor]; exists {
			return errors.New("document anchor is duplicated")
		}
		anchors[block.MarkdownAnchor] = struct{}{}
		previousPlaintextEnd = block.PlaintextUTF8ByteEnd
		previousMarkdownEnd = block.MarkdownUTF8ByteEnd
	}
	if previousPlaintextEnd != int64(len(result.Plaintext)) || result.AnchorMapSHA256 != DocumentAnchorMapSHA256(result) {
		return errors.New("document anchor map coverage or digest is invalid")
	}
	return nil
}

func ValidUTF8DocumentTextRange(value string, start, end int64) bool {
	if !utf8.ValidString(value) || start < 0 || end <= start || end > int64(len(value)) {
		return false
	}
	return utf8RuneBoundary(value, start) && utf8RuneBoundary(value, end)
}

func utf8RuneBoundary(value string, offset int64) bool {
	if offset == 0 || offset == int64(len(value)) {
		return true
	}
	return offset > 0 && offset < int64(len(value)) && utf8.RuneStart(value[offset])
}

func DocumentAnchorMapSHA256(result MapDocumentTextResult) string {
	hash := sha256.New()
	writeAnchorHashString(hash, result.NormalizationVersion)
	writeAnchorHashString(hash, result.AnchorMapProfileVersion)
	writeAnchorHashString(hash, result.PlaintextSHA256)
	writeAnchorHashString(hash, result.MarkdownSHA256)
	for _, block := range result.Blocks {
		writeAnchorHashInt64(hash, int64(block.Ordinal))
		writeAnchorHashInt64(hash, block.PlaintextUTF8ByteStart)
		writeAnchorHashInt64(hash, block.PlaintextUTF8ByteEnd)
		writeAnchorHashInt64(hash, block.MarkdownUTF8ByteStart)
		writeAnchorHashInt64(hash, block.MarkdownUTF8ByteEnd)
		writeAnchorHashString(hash, block.MarkdownAnchor)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// ValidatePersistedDocumentAnchorMap validates the body-free facts crossing
// the Application→Infrastructure persistence boundary.
func ValidatePersistedDocumentAnchorMap(identity *DerivedArtifactAnchorMapDTO, blocks []DocumentAnchorBlockDTO) error {
	if err := validateDerivedArtifactAnchorMapDTO(identity, func() string {
		if identity == nil {
			return ""
		}
		return identity.MarkdownSHA256
	}()); err != nil || len(blocks) == 0 || len(blocks) > maximumDocumentAnchorBlocks {
		return errors.New("persisted document anchor map is invalid")
	}
	anchors := make(map[string]struct{}, len(blocks))
	var previousPlaintextEnd, previousMarkdownEnd int64
	for index, block := range blocks {
		if block.Ordinal != index || block.PlaintextUTF8ByteStart < 0 || block.PlaintextUTF8ByteEnd <= block.PlaintextUTF8ByteStart ||
			block.MarkdownUTF8ByteStart < 0 || block.MarkdownUTF8ByteEnd <= block.MarkdownUTF8ByteStart ||
			!markdownAnchorPattern.MatchString(block.MarkdownAnchor) ||
			(index > 0 && (block.PlaintextUTF8ByteStart <= previousPlaintextEnd || block.MarkdownUTF8ByteStart < previousMarkdownEnd)) {
			return errors.New("persisted document anchor block is invalid")
		}
		if _, exists := anchors[block.MarkdownAnchor]; exists {
			return errors.New("persisted document anchor is duplicated")
		}
		anchors[block.MarkdownAnchor] = struct{}{}
		previousPlaintextEnd = block.PlaintextUTF8ByteEnd
		previousMarkdownEnd = block.MarkdownUTF8ByteEnd
	}
	computed := DocumentAnchorMapSHA256(MapDocumentTextResult{
		NormalizationVersion: identity.NormalizationVersion, AnchorMapProfileVersion: identity.AnchorMapProfileVersion,
		PlaintextSHA256: identity.PlaintextSHA256, MarkdownSHA256: identity.MarkdownSHA256, Blocks: blocks,
	})
	if computed != identity.AnchorMapSHA256 {
		return errors.New("persisted document anchor map digest is invalid")
	}
	return nil
}

func DocumentMarkdownAnchor(ordinal int, plaintextBlock string) string {
	digest := sha256.Sum256([]byte(plaintextBlock))
	return fmt.Sprintf("body-%04d-%x", ordinal, digest[:6])
}

func documentTextSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)
}

func DocumentAnchorMapTextSHA256(value string) string {
	return documentTextSHA256(value)
}

type anchorHashWriter interface {
	Write([]byte) (int, error)
}

func writeAnchorHashString(writer anchorHashWriter, value string) {
	writeAnchorHashInt64(writer, int64(len(value)))
	_, _ = writer.Write([]byte(value))
}

func writeAnchorHashInt64(writer anchorHashWriter, value int64) {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], uint64(value))
	_, _ = writer.Write(buffer[:])
}
