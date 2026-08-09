package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maximumIntentObjectiveLength = 2000
	maximumIntentClauseLength    = 512
	maximumIntentClauses         = 128
	maximumIntentEntities        = 64
	maximumIntentAliases         = 32
	maximumIntentExamples        = 64
	maximumIntentExampleLength   = 4000
)

var ErrInvalidIntent = errors.New("monitor intent is invalid")

type IntentClauseOperator string

const (
	IntentClauseMust    IntentClauseOperator = "must"
	IntentClauseShould  IntentClauseOperator = "should"
	IntentClauseMustNot IntentClauseOperator = "must_not"
)

func (operator IntentClauseOperator) Valid() bool {
	return operator == IntentClauseMust || operator == IntentClauseShould || operator == IntentClauseMustNot
}

type IntentClauseField string

const (
	IntentClauseTerm       IntentClauseField = "term"
	IntentClausePhrase     IntentClauseField = "phrase"
	IntentClauseAction     IntentClauseField = "action"
	IntentClauseLocation   IntentClauseField = "location"
	IntentClauseLanguage   IntentClauseField = "language"
	IntentClauseRegion     IntentClauseField = "region"
	IntentClauseSource     IntentClauseField = "source"
	IntentClauseTimeWindow IntentClauseField = "time_window"
)

func (field IntentClauseField) Valid() bool {
	switch field {
	case IntentClauseTerm, IntentClausePhrase, IntentClauseAction,
		IntentClauseLocation, IntentClauseLanguage, IntentClauseRegion,
		IntentClauseSource, IntentClauseTimeWindow:
		return true
	default:
		return false
	}
}

type IntentExampleLabel string

const (
	IntentExamplePositive IntentExampleLabel = "positive"
	IntentExampleNegative IntentExampleLabel = "negative"
)

func (label IntentExampleLabel) Valid() bool {
	return label == IntentExamplePositive || label == IntentExampleNegative
}

// IntentObjective is the user's natural-language monitoring goal. It is a
// value object: normalization happens once and the value cannot be changed.
type IntentObjective struct {
	value string
}

func NewIntentObjective(value string) (IntentObjective, error) {
	normalized, err := normalizeIntentValue(value, maximumIntentObjectiveLength, "objective")
	if err != nil {
		return IntentObjective{}, err
	}
	return IntentObjective{value: normalized}, nil
}

func (objective IntentObjective) String() string { return objective.value }

// IntentClause represents one declared constraint. Operator and field are
// independent and never inferred from text, so a MUST_NOT action cannot
// silently become a soft term signal.
type IntentClause struct {
	operator IntentClauseOperator
	field    IntentClauseField
	value    string
}

func NewIntentClause(operator IntentClauseOperator, field IntentClauseField, value string) (IntentClause, error) {
	if !operator.Valid() || !field.Valid() {
		return IntentClause{}, fmt.Errorf("%w: intent clause operator or field is invalid", ErrInvalidIntent)
	}
	normalized, err := normalizeIntentValue(value, maximumIntentClauseLength, "clause")
	if err != nil {
		return IntentClause{}, err
	}
	switch field {
	case IntentClauseLanguage:
		languages, languageErr := NormalizeLanguages([]string{normalized}, 1, 1)
		if languageErr != nil {
			return IntentClause{}, fmt.Errorf("%w: %v", ErrInvalidIntent, languageErr)
		}
		normalized = languages[0]
	case IntentClauseRegion:
		regions, regionErr := NormalizeRegions([]string{normalized}, 1, 1)
		if regionErr != nil {
			return IntentClause{}, fmt.Errorf("%w: %v", ErrInvalidIntent, regionErr)
		}
		normalized = regions[0]
	}
	return IntentClause{operator: operator, field: field, value: normalized}, nil
}

func (clause IntentClause) Operator() IntentClauseOperator { return clause.operator }
func (clause IntentClause) Field() IntentClauseField       { return clause.field }
func (clause IntentClause) Value() string                  { return clause.value }

// IntentEntity records an explicit canonical entity and its ambiguity facts.
// Alias collisions are checked across the full IntentDefinition.
type IntentEntity struct {
	canonicalID   string
	displayName   string
	aliases       []string
	ambiguityNote string
}

func NewIntentEntity(canonicalID, displayName string, aliases []string, ambiguityNote string) (IntentEntity, error) {
	id, err := normalizeIntentIdentifier(canonicalID, 256, "canonical entity id")
	if err != nil {
		return IntentEntity{}, err
	}
	display, err := normalizeIntentValue(displayName, 160, "entity display name")
	if err != nil {
		return IntentEntity{}, err
	}
	note := normalizeText(ambiguityNote)
	if strings.ContainsRune(note, '\x00') || utf8.RuneCountInString(note) > 1000 {
		return IntentEntity{}, fmt.Errorf("%w: entity ambiguity note is invalid", ErrInvalidIntent)
	}
	if len(aliases) > maximumIntentAliases {
		return IntentEntity{}, fmt.Errorf("%w: entity has too many aliases", ErrInvalidIntent)
	}
	seen := make(map[string]struct{}, len(aliases)+1)
	seen[canonicalIntentKey(display)] = struct{}{}
	normalizedAliases := make([]string, 0, len(aliases))
	for _, raw := range aliases {
		alias, aliasErr := normalizeIntentValue(raw, 160, "entity alias")
		if aliasErr != nil {
			return IntentEntity{}, aliasErr
		}
		key := canonicalIntentKey(alias)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		normalizedAliases = append(normalizedAliases, alias)
	}
	sort.Slice(normalizedAliases, func(i, j int) bool {
		return canonicalIntentKey(normalizedAliases[i]) < canonicalIntentKey(normalizedAliases[j])
	})
	return IntentEntity{canonicalID: id, displayName: display, aliases: normalizedAliases, ambiguityNote: note}, nil
}

func (entity IntentEntity) CanonicalID() string   { return entity.canonicalID }
func (entity IntentEntity) DisplayName() string   { return entity.displayName }
func (entity IntentEntity) AmbiguityNote() string { return entity.ambiguityNote }
func (entity IntentEntity) Aliases() []string     { return append([]string(nil), entity.aliases...) }

type IntentExample struct {
	label IntentExampleLabel
	text  string
}

func NewIntentExample(label IntentExampleLabel, text string) (IntentExample, error) {
	if !label.Valid() {
		return IntentExample{}, fmt.Errorf("%w: intent example label is invalid", ErrInvalidIntent)
	}
	normalized, err := normalizeIntentValue(text, maximumIntentExampleLength, "example")
	if err != nil {
		return IntentExample{}, err
	}
	return IntentExample{label: label, text: normalized}, nil
}

func (example IntentExample) Label() IntentExampleLabel { return example.label }
func (example IntentExample) Text() string              { return example.text }

// IntentDefinition is the complete atomically replaceable intent payload.
// Its collections are canonicalized to make the fingerprint independent of
// request ordering while preserving every declared business fact.
type IntentDefinition struct {
	objective   IntentObjective
	clauses     []IntentClause
	entities    []IntentEntity
	examples    []IntentExample
	fingerprint string
}

func NewIntentDefinition(objective IntentObjective, clauses []IntentClause, entities []IntentEntity, examples []IntentExample) (IntentDefinition, error) {
	if objective.value == "" {
		return IntentDefinition{}, fmt.Errorf("%w: objective is required", ErrInvalidIntent)
	}
	if len(clauses) > maximumIntentClauses || len(entities) > maximumIntentEntities || len(examples) > maximumIntentExamples {
		return IntentDefinition{}, fmt.Errorf("%w: intent collection limit exceeded", ErrInvalidIntent)
	}
	canonicalClauses, err := validateAndCopyIntentClauses(clauses)
	if err != nil {
		return IntentDefinition{}, err
	}
	canonicalEntities, err := validateAndCopyIntentEntities(entities)
	if err != nil {
		return IntentDefinition{}, err
	}
	canonicalExamples, err := validateAndCopyIntentExamples(examples)
	if err != nil {
		return IntentDefinition{}, err
	}
	definition := IntentDefinition{
		objective: objective,
		clauses:   canonicalClauses,
		entities:  canonicalEntities,
		examples:  canonicalExamples,
	}
	definition.fingerprint = intentDefinitionFingerprint(definition)
	return definition, nil
}

func (definition IntentDefinition) Objective() IntentObjective { return definition.objective }
func (definition IntentDefinition) Clauses() []IntentClause {
	return append([]IntentClause(nil), definition.clauses...)
}
func (definition IntentDefinition) Entities() []IntentEntity {
	return append([]IntentEntity(nil), definition.entities...)
}
func (definition IntentDefinition) Examples() []IntentExample {
	return append([]IntentExample(nil), definition.examples...)
}
func (definition IntentDefinition) Fingerprint() string { return definition.fingerprint }

func validateAndCopyIntentClauses(clauses []IntentClause) ([]IntentClause, error) {
	result := append([]IntentClause(nil), clauses...)
	seen := make(map[string]IntentClauseOperator, len(result))
	positiveTerms := make(map[string]struct{})
	negativeHardTerms := make(map[string]struct{})
	for _, clause := range result {
		if !clause.operator.Valid() || !clause.field.Valid() || clause.value == "" {
			return nil, fmt.Errorf("%w: intent clause is invalid", ErrInvalidIntent)
		}
		valueKey := canonicalIntentKey(clause.value)
		fieldValueKey := string(clause.field) + "\x00" + valueKey
		identity := string(clause.operator) + "\x00" + fieldValueKey
		if _, duplicate := seen[identity]; duplicate {
			return nil, fmt.Errorf("%w: duplicate intent clause", ErrInvalidIntent)
		}
		seen[identity] = clause.operator
		switch clause.operator {
		case IntentClauseMust, IntentClauseShould:
			positiveTerms[fieldValueKey] = struct{}{}
		case IntentClauseMustNot:
			negativeHardTerms[fieldValueKey] = struct{}{}
		}
	}
	for value := range positiveTerms {
		if _, contradiction := negativeHardTerms[value]; contradiction {
			return nil, fmt.Errorf("%w: positive and MUST_NOT clauses contradict", ErrInvalidIntent)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].operator != result[j].operator {
			return result[i].operator < result[j].operator
		}
		if result[i].field != result[j].field {
			return result[i].field < result[j].field
		}
		return canonicalIntentKey(result[i].value) < canonicalIntentKey(result[j].value)
	})
	return result, nil
}

func validateAndCopyIntentEntities(entities []IntentEntity) ([]IntentEntity, error) {
	result := make([]IntentEntity, len(entities))
	canonicalIDs := make(map[string]struct{}, len(entities))
	labels := make(map[string][]int)
	for index, entity := range entities {
		if entity.canonicalID == "" || entity.displayName == "" {
			return nil, fmt.Errorf("%w: intent entity is invalid", ErrInvalidIntent)
		}
		idKey := canonicalIntentKey(entity.canonicalID)
		if _, duplicate := canonicalIDs[idKey]; duplicate {
			return nil, fmt.Errorf("%w: duplicate canonical entity id", ErrInvalidIntent)
		}
		canonicalIDs[idKey] = struct{}{}
		entity.aliases = append([]string(nil), entity.aliases...)
		result[index] = entity
		entityLabels := append([]string{entity.displayName}, entity.aliases...)
		seenForEntity := make(map[string]struct{}, len(entityLabels))
		for _, label := range entityLabels {
			key := canonicalIntentKey(label)
			if _, duplicate := seenForEntity[key]; duplicate {
				continue
			}
			seenForEntity[key] = struct{}{}
			labels[key] = append(labels[key], index)
		}
	}
	for _, indexes := range labels {
		if len(indexes) < 2 {
			continue
		}
		for _, index := range indexes {
			if result[index].ambiguityNote == "" {
				return nil, fmt.Errorf("%w: ambiguous entity label requires an ambiguity note", ErrInvalidIntent)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return canonicalIntentKey(result[i].canonicalID) < canonicalIntentKey(result[j].canonicalID)
	})
	return result, nil
}

func validateAndCopyIntentExamples(examples []IntentExample) ([]IntentExample, error) {
	result := append([]IntentExample(nil), examples...)
	seen := make(map[string]IntentExampleLabel, len(result))
	for _, example := range result {
		if !example.label.Valid() || example.text == "" {
			return nil, fmt.Errorf("%w: intent example is invalid", ErrInvalidIntent)
		}
		key := canonicalIntentKey(example.text)
		if prior, duplicate := seen[key]; duplicate {
			if prior == example.label {
				return nil, fmt.Errorf("%w: duplicate intent example", ErrInvalidIntent)
			}
			return nil, fmt.Errorf("%w: intent example has conflicting labels", ErrInvalidIntent)
		}
		seen[key] = example.label
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].label != result[j].label {
			return result[i].label < result[j].label
		}
		return canonicalIntentKey(result[i].text) < canonicalIntentKey(result[j].text)
	})
	return result, nil
}

func intentDefinitionFingerprint(definition IntentDefinition) string {
	digest := sha256.New()
	writeIntentHashPart(digest, "intent-definition-v1")
	writeIntentHashPart(digest, "objective")
	writeIntentHashPart(digest, definition.objective.value)
	writeIntentHashPart(digest, "clauses")
	writeIntentHashPart(digest, strconv.Itoa(len(definition.clauses)))
	for _, clause := range definition.clauses {
		writeIntentHashPart(digest, string(clause.operator))
		writeIntentHashPart(digest, string(clause.field))
		writeIntentHashPart(digest, clause.value)
	}
	writeIntentHashPart(digest, "entities")
	writeIntentHashPart(digest, strconv.Itoa(len(definition.entities)))
	for _, entity := range definition.entities {
		writeIntentHashPart(digest, entity.canonicalID)
		writeIntentHashPart(digest, entity.displayName)
		writeIntentHashPart(digest, strconv.Itoa(len(entity.aliases)))
		for _, alias := range entity.aliases {
			writeIntentHashPart(digest, alias)
		}
		writeIntentHashPart(digest, entity.ambiguityNote)
	}
	writeIntentHashPart(digest, "examples")
	writeIntentHashPart(digest, strconv.Itoa(len(definition.examples)))
	for _, example := range definition.examples {
		writeIntentHashPart(digest, string(example.label))
		writeIntentHashPart(digest, example.text)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func writeIntentHashPart(target hash.Hash, value string) {
	_, _ = io.WriteString(target, strconv.Itoa(len([]byte(value))))
	_, _ = io.WriteString(target, ":")
	_, _ = io.WriteString(target, value)
	_, _ = io.WriteString(target, "\n")
}

func normalizeIntentValue(value string, maximumRunes int, field string) (string, error) {
	normalized := normalizeText(value)
	if normalized == "" || strings.ContainsRune(normalized, '\x00') || utf8.RuneCountInString(normalized) > maximumRunes {
		return "", fmt.Errorf("%w: %s is invalid", ErrInvalidIntent, field)
	}
	return normalized, nil
}

func normalizeIntentIdentifier(value string, maximumBytes int, field string) (string, error) {
	normalized := normalizeText(value)
	if normalized == "" || strings.ContainsAny(normalized, "\x00\r\n") || len([]byte(normalized)) > maximumBytes {
		return "", fmt.Errorf("%w: %s is invalid", ErrInvalidIntent, field)
	}
	return normalized, nil
}

func canonicalIntentKey(value string) string {
	return strings.ToLower(normalizeText(value))
}
