package application

import (
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

// RawEvidencePartyDTO is an explicit Connector assertion. A missing slice
// means no party fact was supplied; callers must not infer one from URLs,
// author snapshots, or connection metadata.
type RawEvidencePartyDTO struct {
	Role              string
	Kind              string
	IdentityNamespace string
	ExternalID        string
	DisplayName       string
	HomepageURL       string
}

// SourceObservationPartyDTO is the byte-free immutable persistence command
// projection for one evidence-bound observation assertion.
type SourceObservationPartyDTO struct {
	Role              string
	Kind              string
	IdentityNamespace string
	ExternalID        string
	DisplayName       string
	HomepageURL       string
}

func sourcePartyEntitiesFromRawDTOs(values []RawEvidencePartyDTO) ([]domain.SourcePartyAssertion, error) {
	parties := make([]domain.SourcePartyAssertion, len(values))
	for index, value := range values {
		parties[index] = domain.SourcePartyAssertion{
			Role: domain.SourcePartyRole(value.Role), Kind: domain.SourcePartyKind(value.Kind),
			IdentityNamespace: value.IdentityNamespace, ExternalID: value.ExternalID,
			DisplayName: value.DisplayName, HomepageURL: value.HomepageURL,
		}
	}
	normalized, err := domain.NormalizeSourceParties(parties)
	if err != nil {
		return nil, fmt.Errorf("normalize raw evidence parties: %w", err)
	}
	return normalized, nil
}

func rawEvidencePartyDTOsFromEntities(values []domain.SourcePartyAssertion) []RawEvidencePartyDTO {
	result := make([]RawEvidencePartyDTO, len(values))
	for index, value := range values {
		result[index] = RawEvidencePartyDTO{
			Role: string(value.Role), Kind: string(value.Kind), IdentityNamespace: value.IdentityNamespace,
			ExternalID: value.ExternalID, DisplayName: value.DisplayName, HomepageURL: value.HomepageURL,
		}
	}
	return result
}

func sourceObservationPartyDTOsFromEntities(values []domain.SourcePartyAssertion) []SourceObservationPartyDTO {
	result := make([]SourceObservationPartyDTO, len(values))
	for index, value := range values {
		result[index] = SourceObservationPartyDTO{
			Role: string(value.Role), Kind: string(value.Kind), IdentityNamespace: value.IdentityNamespace,
			ExternalID: value.ExternalID, DisplayName: value.DisplayName, HomepageURL: value.HomepageURL,
		}
	}
	return result
}

func sourcePartyEntitiesFromObservationDTOs(values []SourceObservationPartyDTO) ([]domain.SourcePartyAssertion, error) {
	parties := make([]domain.SourcePartyAssertion, len(values))
	for index, value := range values {
		parties[index] = domain.SourcePartyAssertion{
			Role: domain.SourcePartyRole(value.Role), Kind: domain.SourcePartyKind(value.Kind),
			IdentityNamespace: value.IdentityNamespace, ExternalID: value.ExternalID,
			DisplayName: value.DisplayName, HomepageURL: value.HomepageURL,
		}
	}
	return domain.NormalizeSourceParties(parties)
}

func ValidateSourceObservationPartyDTOs(values []SourceObservationPartyDTO) error {
	_, err := sourcePartyEntitiesFromObservationDTOs(values)
	return err
}

func NormalizeSourceObservationPartyDTOs(values []SourceObservationPartyDTO) ([]SourceObservationPartyDTO, error) {
	parties, err := sourcePartyEntitiesFromObservationDTOs(values)
	if err != nil {
		return nil, err
	}
	return sourceObservationPartyDTOsFromEntities(parties), nil
}
