package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const ContentMinHashSize = 64

type ContentRelation string

const (
	ContentRelationExactCopy      ContentRelation = "exact_copy"
	ContentRelationNearDuplicate  ContentRelation = "near_duplicate"
	ContentRelationSyndicatedFrom ContentRelation = "syndicated_from"
	ContentRelationTranslationOf  ContentRelation = "translation_of"
	ContentRelationRevisionOf     ContentRelation = "revision_of"
	ContentRelationUnrelated      ContentRelation = "unrelated"
)

func (relation ContentRelation) Valid() bool {
	switch relation {
	case ContentRelationExactCopy, ContentRelationNearDuplicate, ContentRelationSyndicatedFrom,
		ContentRelationTranslationOf, ContentRelationRevisionOf, ContentRelationUnrelated:
		return true
	default:
		return false
	}
}

func (relation ContentRelation) Symmetric() bool {
	return relation == ContentRelationExactCopy || relation == ContentRelationNearDuplicate || relation == ContentRelationUnrelated
}

type ContentFingerprint struct {
	ProfileVersion          string
	NormalizedContentSHA256 string
	SimHashHex              string
	MinHash                 []uint64
}

func (fingerprint ContentFingerprint) Validate() error {
	if strings.TrimSpace(fingerprint.ProfileVersion) == "" || len(fingerprint.ProfileVersion) > 64 ||
		!validSHA256(fingerprint.NormalizedContentSHA256) || len(fingerprint.SimHashHex) != 16 ||
		strings.ToLower(fingerprint.SimHashHex) != fingerprint.SimHashHex || len(fingerprint.MinHash) != ContentMinHashSize {
		return fmt.Errorf("invalid content fingerprint")
	}
	if _, err := strconv.ParseUint(fingerprint.SimHashHex, 16, 64); err != nil {
		return fmt.Errorf("invalid content fingerprint")
	}
	return nil
}

func BuildContentFingerprint(text, profileVersion string) (ContentFingerprint, error) {
	normalized := normalizeFingerprintText(text)
	if normalized == "" || strings.TrimSpace(profileVersion) == "" || len(profileVersion) > 64 {
		return ContentFingerprint{}, fmt.Errorf("content text and profile version are required")
	}
	digest := sha256.Sum256([]byte(normalized))
	tokens := fingerprintTokens(normalized)
	result := ContentFingerprint{
		ProfileVersion: strings.TrimSpace(profileVersion), NormalizedContentSHA256: hex.EncodeToString(digest[:]),
		SimHashHex: fmt.Sprintf("%016x", contentSimHash(tokens)), MinHash: contentMinHash(tokens),
	}
	if err := result.Validate(); err != nil {
		return ContentFingerprint{}, err
	}
	return result, nil
}

func normalizeFingerprintText(value string) string {
	value = norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n"))
	return strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
}

func fingerprintTokens(value string) []string {
	normalized := []rune(strings.ToLower(value))
	if len(normalized) <= 5 {
		return []string{string(normalized)}
	}
	tokens := make([]string, 0, len(normalized)-4)
	for start := 0; start+5 <= len(normalized); start++ {
		tokens = append(tokens, string(normalized[start:start+5]))
	}
	return tokens
}

func contentSimHash(tokens []string) uint64 {
	var weights [64]int
	for _, token := range tokens {
		digest := sha256.Sum256([]byte(token))
		value := binary.BigEndian.Uint64(digest[:8])
		for bit := 0; bit < 64; bit++ {
			if value&(uint64(1)<<bit) != 0 {
				weights[bit]++
			} else {
				weights[bit]--
			}
		}
	}
	var result uint64
	for bit, weight := range weights {
		if weight >= 0 {
			result |= uint64(1) << bit
		}
	}
	return result
}

func contentMinHash(tokens []string) []uint64 {
	unique := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		unique[token] = struct{}{}
	}
	result := make([]uint64, ContentMinHashSize)
	for index := range result {
		result[index] = math.MaxUint64
		seed := make([]byte, 8)
		binary.BigEndian.PutUint64(seed, uint64(index+1))
		for token := range unique {
			digest := sha256.Sum256(append(seed, []byte(token)...))
			value := binary.BigEndian.Uint64(digest[:8])
			if value < result[index] {
				result[index] = value
			}
		}
	}
	return result
}

type ContentFamilyAction string

const (
	ContentFamilyActionCreate ContentFamilyAction = "create"
	ContentFamilyActionJoin   ContentFamilyAction = "join"
	ContentFamilyActionReview ContentFamilyAction = "review"
)

type ContentFamilyCandidate struct {
	FamilyID              int64
	FamilyVersion         int64
	RootDocumentVersionID int64
	Fingerprint           ContentFingerprint
}

type ContentFamilyDecisionInput struct {
	DocumentVersionID      int64
	Fingerprint            ContentFingerprint
	Candidates             []ContentFamilyCandidate
	DecisionProfileVersion string
	HardConflict           bool
}

type ContentFamilyDecision struct {
	Action                 ContentFamilyAction
	FamilyID               int64
	FamilyVersion          int64
	RootDocumentVersionID  int64
	Relation               ContentRelation
	HammingDistance        int
	MinHashSimilarity      float64
	DecisionProfileVersion string
	ReasonCodes            []string
}

func DecideContentFamily(input ContentFamilyDecisionInput) (ContentFamilyDecision, error) {
	if input.DocumentVersionID <= 0 || strings.TrimSpace(input.DecisionProfileVersion) == "" || len(input.DecisionProfileVersion) > 64 {
		return ContentFamilyDecision{}, fmt.Errorf("invalid content family decision input")
	}
	if err := input.Fingerprint.Validate(); err != nil {
		return ContentFamilyDecision{}, err
	}
	if input.HardConflict {
		return ContentFamilyDecision{Action: ContentFamilyActionCreate, Relation: ContentRelationUnrelated, DecisionProfileVersion: input.DecisionProfileVersion, ReasonCodes: []string{"hard_conflict"}}, nil
	}
	type scoredCandidate struct {
		candidate  ContentFamilyCandidate
		hamming    int
		similarity float64
	}
	scored := make([]scoredCandidate, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		if candidate.FamilyID <= 0 || candidate.FamilyVersion <= 0 || candidate.RootDocumentVersionID <= 0 || candidate.RootDocumentVersionID == input.DocumentVersionID {
			return ContentFamilyDecision{}, fmt.Errorf("invalid content family candidate")
		}
		if err := candidate.Fingerprint.Validate(); err != nil || candidate.Fingerprint.ProfileVersion != input.Fingerprint.ProfileVersion {
			return ContentFamilyDecision{}, fmt.Errorf("invalid content family candidate fingerprint")
		}
		if candidate.Fingerprint.NormalizedContentSHA256 == input.Fingerprint.NormalizedContentSHA256 {
			return contentFamilyCandidateDecision(candidate, ContentFamilyActionJoin, ContentRelationExactCopy, 0, 1, input.DecisionProfileVersion, "exact_content_hash"), nil
		}
		hamming, err := simHashDistance(input.Fingerprint.SimHashHex, candidate.Fingerprint.SimHashHex)
		if err != nil {
			return ContentFamilyDecision{}, err
		}
		scored = append(scored, scoredCandidate{candidate: candidate, hamming: hamming, similarity: minHashSimilarity(input.Fingerprint.MinHash, candidate.Fingerprint.MinHash)})
	}
	sort.SliceStable(scored, func(left, right int) bool {
		if scored[left].hamming != scored[right].hamming {
			return scored[left].hamming < scored[right].hamming
		}
		if scored[left].similarity != scored[right].similarity {
			return scored[left].similarity > scored[right].similarity
		}
		return scored[left].candidate.FamilyID < scored[right].candidate.FamilyID
	})
	if len(scored) == 0 {
		return ContentFamilyDecision{Action: ContentFamilyActionCreate, Relation: ContentRelationUnrelated, DecisionProfileVersion: input.DecisionProfileVersion, ReasonCodes: []string{"no_candidate"}}, nil
	}
	best := scored[0]
	if best.hamming <= 3 && best.similarity >= 0.90 {
		return contentFamilyCandidateDecision(best.candidate, ContentFamilyActionJoin, ContentRelationNearDuplicate, best.hamming, best.similarity, input.DecisionProfileVersion, "conservative_near_duplicate"), nil
	}
	if best.hamming <= 6 || best.similarity >= 0.75 {
		return contentFamilyCandidateDecision(best.candidate, ContentFamilyActionReview, ContentRelationNearDuplicate, best.hamming, best.similarity, input.DecisionProfileVersion, "ambiguous_similarity"), nil
	}
	return ContentFamilyDecision{Action: ContentFamilyActionCreate, Relation: ContentRelationUnrelated, HammingDistance: best.hamming, MinHashSimilarity: best.similarity, DecisionProfileVersion: input.DecisionProfileVersion, ReasonCodes: []string{"below_duplicate_threshold"}}, nil
}

func contentFamilyCandidateDecision(candidate ContentFamilyCandidate, action ContentFamilyAction, relation ContentRelation, hamming int, similarity float64, profileVersion, reason string) ContentFamilyDecision {
	return ContentFamilyDecision{Action: action, FamilyID: candidate.FamilyID, FamilyVersion: candidate.FamilyVersion, RootDocumentVersionID: candidate.RootDocumentVersionID, Relation: relation, HammingDistance: hamming, MinHashSimilarity: similarity, DecisionProfileVersion: profileVersion, ReasonCodes: []string{reason}}
}

func simHashDistance(left, right string) (int, error) {
	leftValue, leftErr := strconv.ParseUint(left, 16, 64)
	rightValue, rightErr := strconv.ParseUint(right, 16, 64)
	if leftErr != nil || rightErr != nil {
		return 0, fmt.Errorf("invalid SimHash")
	}
	return bits.OnesCount64(leftValue ^ rightValue), nil
}

func minHashSimilarity(left, right []uint64) float64 {
	if len(left) != ContentMinHashSize || len(right) != ContentMinHashSize {
		return 0
	}
	equal := 0
	for index := range left {
		if left[index] == right[index] {
			equal++
		}
	}
	return float64(equal) / ContentMinHashSize
}

func equalMinHash(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
