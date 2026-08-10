// Package evidence provides the connector-neutral selector used both before
// raw archive and after retrieval. Provider-specific XML replay remains in the
// RSS adapter; RFC 6901 JSON replay is shared by API connectors.
package evidence

import (
	"errors"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/evidencecapture"
	sourcerss "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/rss"
)

type Selector struct {
	feed sourcerss.EvidenceSelectorVerifier
}

func NewSelector() Selector {
	return Selector{feed: sourcerss.NewEvidenceSelectorVerifier()}
}

func (selector Selector) Verify(input sourceapplication.EvidenceSelectorInputDTO) error {
	_, err := selector.Select(input)
	return err
}

func (selector Selector) Select(input sourceapplication.EvidenceSelectorInputDTO) ([]byte, error) {
	if err := input.Validate(); err != nil {
		return nil, errors.New("evidence selector input is invalid")
	}
	if input.Reference.LocatorType != "json_pointer" {
		return selector.feed.Select(input)
	}
	if input.Reference.SelectorVersion != evidencecapture.JSONPointerSelectorVersion {
		return nil, errors.New("JSON selector version is unsupported")
	}
	selected, err := evidencecapture.SelectJSONPointer(input.Snapshot.Payload, input.Reference.LocatorValue)
	if err != nil {
		return nil, err
	}
	return sourcerss.VerifySelectedPayloadDigest(selected, input.Reference.SelectedPayloadSHA256)
}
