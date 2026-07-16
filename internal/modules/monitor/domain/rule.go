package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type RuleType string
type RuleOperator string
type RuleOrigin string
type RuleApprovalStatus string

const (
	RuleTypeKeyword        RuleType = "keyword"
	RuleTypePhrase         RuleType = "phrase"
	RuleTypeExcludeKeyword RuleType = "exclude_keyword"
	RuleTypeDomain         RuleType = "domain"
	RuleTypeAuthor         RuleType = "author"
	RuleTypeLanguage       RuleType = "language"
	RuleTypeRegion         RuleType = "region"
	RuleTypeEntity         RuleType = "entity"
	RuleTypeRegex          RuleType = "regex"

	RuleOperatorContains  RuleOperator = "contains"
	RuleOperatorEquals    RuleOperator = "equals"
	RuleOperatorNotEquals RuleOperator = "not_equals"
	RuleOperatorMatches   RuleOperator = "matches"

	RuleOriginUser   RuleOrigin = "user"
	RuleOriginAI     RuleOrigin = "ai"
	RuleOriginSystem RuleOrigin = "system"

	RuleApprovalPending  RuleApprovalStatus = "pending"
	RuleApprovalApproved RuleApprovalStatus = "approved"
	RuleApprovalRejected RuleApprovalStatus = "rejected"
)

type MonitorRule struct {
	ID              int64
	Version         int64
	ConfigVersionID int64
	RuleType        RuleType
	Operator        RuleOperator
	Value           string
	Weight          float64
	Priority        int16
	Origin          RuleOrigin
	ApprovalStatus  RuleApprovalStatus
	Enabled         bool
}

type MonitorSource struct {
	ID                 int64
	Version            int64
	ConfigVersionID    int64
	SourceConnectionID int64
	QueryOverride      string
	QuerySignature     string
	Priority           int16
	Enabled            bool
}

func NewRule(ruleType RuleType, operator RuleOperator, value string, weight float64, priority int16, origin RuleOrigin) (MonitorRule, error) {
	approval := RuleApprovalApproved
	if origin == RuleOriginAI {
		approval = RuleApprovalPending
	}
	return NormalizeRule(MonitorRule{
		RuleType: ruleType, Operator: operator, Value: value, Weight: weight,
		Priority: priority, Origin: origin, ApprovalStatus: approval, Enabled: true,
	})
}

func NormalizeRule(rule MonitorRule) (MonitorRule, error) {
	if !validRuleOrigin(rule.Origin) || !validApproval(rule.ApprovalStatus) {
		return MonitorRule{}, fmt.Errorf("rule origin or approval status is invalid")
	}
	if !ruleMatrixAllows(rule.RuleType, rule.Operator) {
		return MonitorRule{}, fmt.Errorf("rule type %q does not allow operator %q", rule.RuleType, rule.Operator)
	}
	value, err := normalizeRuleValue(rule.RuleType, rule.Value)
	if err != nil {
		return MonitorRule{}, err
	}
	rule.Value = value
	if rule.Weight < 0 || rule.Weight > 100 {
		return MonitorRule{}, fmt.Errorf("rule weight must be from 0 to 100")
	}
	if fixedZeroWeight(rule.RuleType) && rule.Weight != 0 {
		return MonitorRule{}, fmt.Errorf("rule type %q requires zero weight", rule.RuleType)
	}
	return rule, nil
}

func HasApprovedHumanCoreRule(rules []MonitorRule) bool {
	for _, rule := range rules {
		if rule.Enabled && rule.ApprovalStatus == RuleApprovalApproved && (rule.Origin == RuleOriginUser || rule.Origin == RuleOriginSystem) &&
			(rule.RuleType == RuleTypeKeyword || rule.RuleType == RuleTypePhrase || rule.RuleType == RuleTypeEntity) {
			return true
		}
	}
	return false
}

func NormalizeQueryOverride(value string) (string, error) {
	normalized := normalizeText(value)
	if strings.ContainsRune(normalized, '\x00') {
		return "", fmt.Errorf("query override cannot contain NUL")
	}
	if normalized == "" {
		return "", nil
	}
	if length := len([]byte(normalized)); length < 1 || length > 2048 {
		return "", fmt.Errorf("query override must be 1-2048 UTF-8 bytes")
	}
	return normalized, nil
}

func normalizeRuleValue(ruleType RuleType, value string) (string, error) {
	normalized := normalizeText(value)
	if normalized == "" || strings.ContainsRune(normalized, '\x00') {
		return "", fmt.Errorf("rule value is required")
	}
	switch ruleType {
	case RuleTypeKeyword, RuleTypePhrase, RuleTypeEntity, RuleTypeExcludeKeyword:
		if utf8.RuneCountInString(normalized) > 160 {
			return "", fmt.Errorf("rule value must be at most 160 characters")
		}
	case RuleTypeDomain:
		if len(normalized) > 253 || !validDomain(normalized) {
			return "", fmt.Errorf("rule domain is invalid")
		}
		normalized = strings.ToLower(strings.TrimSuffix(normalized, "."))
	case RuleTypeAuthor:
		if utf8.RuneCountInString(normalized) > 128 {
			return "", fmt.Errorf("author rule value must be at most 128 characters")
		}
	case RuleTypeLanguage:
		values, err := NormalizeLanguages([]string{normalized}, 1, 1)
		if err != nil {
			return "", err
		}
		normalized = values[0]
	case RuleTypeRegion:
		values, err := NormalizeRegions([]string{normalized}, 1, 1)
		if err != nil {
			return "", err
		}
		normalized = values[0]
	case RuleTypeRegex:
		if length := len([]byte(normalized)); length > 256 {
			return "", fmt.Errorf("regex rule value must be at most 256 UTF-8 bytes")
		}
		if _, err := regexp.Compile(normalized); err != nil {
			return "", fmt.Errorf("regex rule value is invalid: %w", err)
		}
	default:
		return "", fmt.Errorf("rule type %q is invalid", ruleType)
	}
	return normalized, nil
}

func ruleMatrixAllows(ruleType RuleType, operator RuleOperator) bool {
	switch ruleType {
	case RuleTypeKeyword, RuleTypePhrase, RuleTypeEntity, RuleTypeExcludeKeyword:
		return operator == RuleOperatorContains || operator == RuleOperatorEquals
	case RuleTypeDomain, RuleTypeAuthor, RuleTypeLanguage, RuleTypeRegion:
		return operator == RuleOperatorEquals || operator == RuleOperatorNotEquals
	case RuleTypeRegex:
		return operator == RuleOperatorMatches
	default:
		return false
	}
}

func fixedZeroWeight(ruleType RuleType) bool {
	return ruleType == RuleTypeExcludeKeyword || ruleType == RuleTypeDomain || ruleType == RuleTypeAuthor || ruleType == RuleTypeLanguage || ruleType == RuleTypeRegion
}

func validRuleOrigin(origin RuleOrigin) bool {
	return origin == RuleOriginUser || origin == RuleOriginAI || origin == RuleOriginSystem
}

func validApproval(approval RuleApprovalStatus) bool {
	return approval == RuleApprovalPending || approval == RuleApprovalApproved || approval == RuleApprovalRejected
}

func validDomain(value string) bool {
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	if value == "" || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return false
			}
		}
	}
	return true
}

type canonicalRule struct {
	ID       int64              `json:"id"`
	RuleType RuleType           `json:"rule_type"`
	Operator RuleOperator       `json:"operator"`
	Value    string             `json:"value"`
	Weight   float64            `json:"weight"`
	Priority int16              `json:"priority"`
	Origin   RuleOrigin         `json:"origin"`
	Approval RuleApprovalStatus `json:"approval_status"`
	Enabled  bool               `json:"enabled"`
}

type canonicalSource struct {
	ID                 int64    `json:"id"`
	SourceConnectionID int64    `json:"source_connection_id"`
	QueryOverride      string   `json:"query_override"`
	Priority           int16    `json:"priority"`
	Enabled            bool     `json:"enabled"`
	SourceOptions      struct{} `json:"source_options"`
}

func canonicalRules(rules []MonitorRule) ([]canonicalRule, error) {
	result := make([]canonicalRule, 0, len(rules))
	for _, rule := range rules {
		normalized, err := NormalizeRule(rule)
		if err != nil {
			return nil, err
		}
		result = append(result, canonicalRule{ID: normalized.ID, RuleType: normalized.RuleType, Operator: normalized.Operator, Value: normalized.Value, Weight: normalized.Weight, Priority: normalized.Priority, Origin: normalized.Origin, Approval: normalized.ApprovalStatus, Enabled: normalized.Enabled})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.RuleType != right.RuleType {
			return left.RuleType < right.RuleType
		}
		if left.Operator != right.Operator {
			return left.Operator < right.Operator
		}
		if left.Value != right.Value {
			return left.Value < right.Value
		}
		if left.Weight != right.Weight {
			return left.Weight < right.Weight
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		if left.Origin != right.Origin {
			return left.Origin < right.Origin
		}
		if left.Approval != right.Approval {
			return left.Approval < right.Approval
		}
		return left.ID < right.ID
	})
	return result, nil
}

func canonicalSources(sources []MonitorSource) ([]canonicalSource, error) {
	result := make([]canonicalSource, 0, len(sources))
	for _, source := range sources {
		if source.SourceConnectionID <= 0 {
			return nil, fmt.Errorf("source connection id must be positive")
		}
		override, err := NormalizeQueryOverride(source.QueryOverride)
		if err != nil {
			return nil, err
		}
		result = append(result, canonicalSource{ID: source.ID, SourceConnectionID: source.SourceConnectionID, QueryOverride: override, Priority: source.Priority, Enabled: source.Enabled})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.SourceConnectionID != right.SourceConnectionID {
			return left.SourceConnectionID < right.SourceConnectionID
		}
		if left.QueryOverride != right.QueryOverride {
			return left.QueryOverride < right.QueryOverride
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.ID < right.ID
	})
	return result, nil
}
