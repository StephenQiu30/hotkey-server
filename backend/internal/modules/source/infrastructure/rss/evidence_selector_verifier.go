package rss

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
)

// EvidenceSelectorVerifier replays only frozen selector versions against the
// raw snapshot bytes. Unsupported selectors fail closed.
type EvidenceSelectorVerifier struct{}

func NewEvidenceSelectorVerifier() EvidenceSelectorVerifier {
	return EvidenceSelectorVerifier{}
}

func (EvidenceSelectorVerifier) Verify(input sourceapplication.EvidenceSelectorInputDTO) error {
	_, err := (EvidenceSelectorVerifier{}).Select(input)
	return err
}

// Select independently replays a frozen locator and returns a defensive copy
// only after the declared selected-payload SHA-256 has been verified.
func (EvidenceSelectorVerifier) Select(input sourceapplication.EvidenceSelectorInputDTO) ([]byte, error) {
	if err := input.Validate(); err != nil {
		return nil, errors.New("evidence selector input is invalid")
	}
	snapshot, reference := input.Snapshot, input.Reference

	var selected []byte
	var err error
	switch reference.LocatorType {
	case "whole_payload":
		if reference.SelectorVersion != "whole-payload-sha256-v1" || reference.LocatorValue != "/" {
			return nil, errors.New("whole-payload selector version or locator is unsupported")
		}
		selected = snapshot.Payload
	case "byte_range":
		if reference.SelectorVersion != "byte-range-sha256-v1" || reference.ByteStart == nil || reference.ByteEnd == nil {
			return nil, errors.New("byte-range selector version is unsupported")
		}
		if reference.LocatorValue != fmt.Sprintf("bytes[%d:%d]", *reference.ByteStart, *reference.ByteEnd) {
			return nil, errors.New("byte-range locator is not canonical")
		}
		if *reference.ByteStart < 0 || *reference.ByteEnd <= *reference.ByteStart || *reference.ByteEnd > int64(len(snapshot.Payload)) {
			return nil, errors.New("byte-range locator is outside the raw payload")
		}
		selected = snapshot.Payload[*reference.ByteStart:*reference.ByteEnd]
	case "xml_path":
		selected, err = selectXMLItem(snapshot.Payload, reference.LocatorValue, reference.SelectorVersion)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("evidence locator type is unsupported")
	}

	digest := sha256.Sum256(selected)
	declared, err := hex.DecodeString(reference.SelectedPayloadSHA256)
	if err != nil || len(declared) != sha256.Size || subtle.ConstantTimeCompare(digest[:], declared) != 1 {
		return nil, errors.New("selected evidence SHA-256 does not match raw bytes")
	}
	return append([]byte(nil), selected...), nil
}

func selectXMLItem(payload []byte, locator, selectorVersion string) ([]byte, error) {
	var selected any
	switch selectorVersion {
	case RSSItemSelectorVersion:
		index, err := selectorIndex(locator, "/rss/channel/item[")
		if err != nil {
			return nil, err
		}
		var document rssDocument
		if err := xml.Unmarshal(payload, &document); err != nil || index > len(document.Channel.Items) {
			return nil, errors.New("RSS item selector did not resolve")
		}
		selected = document.Channel.Items[index-1]
	case RDFItemSelectorVersion:
		index, err := selectorIndex(locator, "/rdf:RDF/item[")
		if err != nil {
			return nil, err
		}
		var document rdfDocument
		if err := xml.Unmarshal(payload, &document); err != nil || index > len(document.Items) {
			return nil, errors.New("RDF item selector did not resolve")
		}
		selected = document.Items[index-1]
	case AtomEntrySelectorVersion:
		index, err := selectorIndex(locator, "/feed/entry[")
		if err != nil {
			return nil, err
		}
		var document atomFeed
		if err := xml.Unmarshal(payload, &document); err != nil || index > len(document.Entries) {
			return nil, errors.New("Atom entry selector did not resolve")
		}
		selected = document.Entries[index-1]
	default:
		return nil, errors.New("XML selector version is unsupported")
	}
	encoded, err := xml.Marshal(selected)
	if err != nil {
		return nil, errors.New("selected XML evidence could not be encoded")
	}
	return encoded, nil
}

func selectorIndex(locator, prefix string) (int, error) {
	if !strings.HasPrefix(locator, prefix) || !strings.HasSuffix(locator, "]") {
		return 0, errors.New("XML selector locator is invalid")
	}
	value := strings.TrimSuffix(strings.TrimPrefix(locator, prefix), "]")
	index, err := strconv.Atoi(value)
	if err != nil || index <= 0 || strconv.Itoa(index) != value {
		return 0, errors.New("XML selector locator is invalid")
	}
	return index, nil
}
