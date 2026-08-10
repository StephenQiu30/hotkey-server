// Package evidencecapture builds immutable evidence snapshots and replays
// provider-neutral JSON Pointer selectors. It never interprets business
// fields: Connector adapters remain responsible for mapping provider records.
package evidencecapture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

const JSONPointerSelectorVersion = "rfc6901-json-number-preserving-v1"

// NewHTTPSnapshot converts a successful bounded HTTP response into a raw
// snapshot. The default MIME is an explicit Connector contract because some
// official APIs omit Content-Type in test and edge responses; allowlisted
// response headers remain exact and are never synthesized.
func NewHTTPSnapshot(
	payload []byte,
	defaultMIMEType string,
	collectorProfileVersion string,
	requestedURL string,
	finalURL string,
	redirectChain []string,
	statusCode int,
	responseHeaders http.Header,
	capturedAt time.Time,
) (domain.EvidenceSnapshot, error) {
	defaultMIMEType = strings.TrimSpace(strings.ToLower(defaultMIMEType))
	if defaultMIMEType == "" {
		return domain.EvidenceSnapshot{}, errors.New("default evidence MIME type is required")
	}
	profile, err := domain.NewCollectorProfileVersion(collectorProfileVersion)
	if err != nil {
		return domain.EvidenceSnapshot{}, err
	}
	headers, err := domain.NewRawResponseHeaders(map[string][]string(responseHeaders))
	if err != nil {
		return domain.EvidenceSnapshot{}, err
	}
	mimeType := defaultMIMEType
	if values := headers.Values()["Content-Type"]; len(values) == 1 {
		mimeType = values[0]
	}
	return domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
		Payload: append([]byte(nil), payload...), CollectorProfileVersion: profile,
		MIMEType: mimeType, StatusCode: statusCode, RequestedURL: requestedURL,
		FinalURL: finalURL, RedirectChain: append([]string(nil), redirectChain...),
		ResponseHeaders: headers, CapturedAt: capturedAt.UTC(),
	})
}

// NewJSONSnapshot is the semantic constructor used by official JSON APIs.
func NewJSONSnapshot(
	payload []byte,
	collectorProfileVersion string,
	requestedURL string,
	finalURL string,
	redirectChain []string,
	statusCode int,
	responseHeaders http.Header,
	capturedAt time.Time,
) (domain.EvidenceSnapshot, error) {
	return NewHTTPSnapshot(payload, "application/json", collectorProfileVersion, requestedURL, finalURL,
		redirectChain, statusCode, responseHeaders, capturedAt)
}

// BindJSONPointer attaches one exact JSON subtree to a normalized SourceItem.
// The selected digest is computed by replaying the same selector used during
// archive and read verification, rather than by marshaling a reduced provider
// struct that may have discarded unknown response fields.
func BindJSONPointer(item *domain.SourceItem, snapshot domain.EvidenceSnapshot, pointer string, usage domain.EvidenceUsage) error {
	if item == nil || !snapshot.VerifyPayload() || (usage != domain.EvidenceUsageDocumentSource && usage != domain.EvidenceUsageContext) {
		return errors.New("JSON evidence binding is invalid")
	}
	selected, err := SelectJSONPointer(snapshot.Payload, pointer)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(selected)
	reference := domain.EvidenceReference{
		SnapshotKey: snapshot.Key, Usage: usage, LocatorType: domain.EvidenceLocatorJSONPointer,
		LocatorValue: pointer, SelectedPayloadSHA256: hex.EncodeToString(digest[:]),
		SelectorVersion: JSONPointerSelectorVersion,
	}
	if err := reference.Validate(); err != nil {
		return err
	}
	item.EvidenceReferences = append(item.EvidenceReferences, reference)
	if len(item.EvidenceReferences) == 1 {
		item.SnapshotKey = snapshot.Key
		item.ItemLocator = pointer
	}
	return nil
}

func SelectJSONPointer(payload []byte, pointer string) ([]byte, error) {
	if pointer == "" || !strings.HasPrefix(pointer, "/") || len(pointer) > 2048 || strings.ContainsAny(pointer, "\x00\r\n") {
		return nil, errors.New("JSON Pointer is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var current any
	if err := decoder.Decode(&current); err != nil {
		return nil, errors.New("JSON evidence payload is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON evidence payload has trailing or invalid values")
	}
	for _, encoded := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		segment, err := decodePointerSegment(encoded)
		if err != nil {
			return nil, err
		}
		switch value := current.(type) {
		case map[string]any:
			selected, found := value[segment]
			if !found {
				return nil, errors.New("JSON Pointer did not resolve")
			}
			current = selected
		case []any:
			if segment == "" || (len(segment) > 1 && segment[0] == '0') {
				return nil, errors.New("JSON Pointer array index is not canonical")
			}
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(value) {
				return nil, errors.New("JSON Pointer did not resolve")
			}
			current = value[index]
		default:
			return nil, errors.New("JSON Pointer traverses a scalar")
		}
	}
	selected, err := json.Marshal(current)
	if err != nil {
		return nil, fmt.Errorf("encode selected JSON evidence: %w", err)
	}
	return selected, nil
}

func decodePointerSegment(value string) (string, error) {
	var result strings.Builder
	result.Grow(len(value))
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			result.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) {
			return "", errors.New("JSON Pointer escape is invalid")
		}
		index++
		switch value[index] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", errors.New("JSON Pointer escape is invalid")
		}
	}
	return result.String(), nil
}

// RedirectChain reconstructs the requests followed by net/http. The final
// destination is always the last value when it differs from requestedURL.
func RedirectChain(requestedURL string, finalRequest *http.Request) []string {
	if finalRequest == nil || finalRequest.URL == nil || finalRequest.URL.String() == requestedURL {
		return []string{}
	}
	reversed := make([]string, 0, 3)
	for request := finalRequest; request != nil && request.URL != nil && request.URL.String() != requestedURL; {
		reversed = append(reversed, request.URL.String())
		if request.Response == nil {
			break
		}
		request = request.Response.Request
	}
	result := make([]string, len(reversed))
	for index := range reversed {
		result[len(reversed)-1-index] = reversed[index]
	}
	return result
}
