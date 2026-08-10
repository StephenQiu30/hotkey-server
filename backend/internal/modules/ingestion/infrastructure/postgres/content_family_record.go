package postgres

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

type contentFamilyCandidateRecord struct {
	familyID, familyVersion, rootDocumentVersionID int64
	profileVersion, normalizedSHA256, simHashHex   string
	minHashBytes                                   []byte
}

func (record contentFamilyCandidateRecord) dto() (ingestionapplication.ContentFamilyCandidateDTO, error) {
	minHash, err := decodeContentMinHash(record.minHashBytes)
	if err != nil {
		return ingestionapplication.ContentFamilyCandidateDTO{}, err
	}
	return ingestionapplication.ContentFamilyCandidateDTO{
		FamilyID: record.familyID, FamilyVersion: record.familyVersion, RootDocumentVersionID: record.rootDocumentVersionID,
		Fingerprint: ingestionapplication.ContentFingerprintDTO{ProfileVersion: record.profileVersion,
			NormalizedContentSHA256: strings.TrimSpace(record.normalizedSHA256), SimHashHex: strings.TrimSpace(record.simHashHex), MinHash: minHash},
	}, nil
}

type contentFamilyDecisionRecord struct {
	decisionID, familyID, familyVersion, documentVersionID, rootDocumentVersionID int64
	action, relation, decisionProfileVersion                                      string
	hammingDistance                                                               int
	minHashSimilarity                                                             float64
	reasonCodesJSON                                                               []byte
	commandFingerprint                                                            string
}

func (record contentFamilyDecisionRecord) dto() (ingestionapplication.ContentFamilyDecisionDTO, error) {
	reasons := []string{}
	if err := json.Unmarshal(record.reasonCodesJSON, &reasons); err != nil || len(reasons) == 0 {
		return ingestionapplication.ContentFamilyDecisionDTO{}, fmt.Errorf("invalid content lineage reason codes")
	}
	return ingestionapplication.ContentFamilyDecisionDTO{DecisionID: record.decisionID, FamilyID: record.familyID,
		FamilyVersion: record.familyVersion, DocumentVersionID: record.documentVersionID,
		RootDocumentVersionID: record.rootDocumentVersionID, Action: record.action, Relation: record.relation,
		HammingDistance: record.hammingDistance, MinHashSimilarity: record.minHashSimilarity,
		DecisionProfileVersion: record.decisionProfileVersion, ReasonCodes: reasons}, nil
}

func encodeContentMinHash(values []uint64) ([]byte, error) {
	if len(values) != 64 {
		return nil, fmt.Errorf("content MinHash must have 64 values")
	}
	result := make([]byte, len(values)*8)
	for index, value := range values {
		binary.BigEndian.PutUint64(result[index*8:], value)
	}
	return result, nil
}

func decodeContentMinHash(value []byte) ([]uint64, error) {
	if len(value) != 512 {
		return nil, fmt.Errorf("content MinHash must occupy 512 bytes")
	}
	result := make([]uint64, 64)
	for index := range result {
		result[index] = binary.BigEndian.Uint64(value[index*8:])
	}
	return result, nil
}
