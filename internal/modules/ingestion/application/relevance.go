package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"unicode"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	"golang.org/x/text/unicode/norm"
)

const (
	relevanceVersion         = "relevance-v1"
	relevanceDegradedVersion = "relevance-v1-degraded"
	maximumLexicalTerms      = 512
)

func validRelevanceContent(content RelevanceContent) bool {
	return content.ID > 0 && content.SourceConnectionID > 0 && len(content.DedupeKey) == 64 && strings.TrimSpace(content.Language) != ""
}

func scoreRelevanceCandidate(request RelevanceScoreRequest, candidate ingestiondomain.RelevanceCandidate, hit mergedCandidateHit, vectorProfile *ModelProfileReference) (ScoredRelevanceCandidate, error) {
	if candidate.MonitorID <= 0 || candidate.MonitorConfigVersionID <= 0 || len(candidate.ConfigHash) != 64 || candidate.RelevanceThreshold < 60 || candidate.RelevanceThreshold > 100 {
		return ScoredRelevanceCandidate{}, fmt.Errorf("invalid published relevance candidate")
	}
	paths := recallPaths(hit)
	rules := approvedRules(candidate.Rules)
	lexical, entities := splitWeightedRules(rules)
	lexicalScore, matchedTerms, titleScore := termScores(request.Content, lexical)
	entityScore, matchedEntities, _ := termScores(request.Content, entities)
	hard, excluded, hardReasons := hardVeto(request.Content, candidate, rules, lexicalScore > 0, entityScore > 0)
	preference := preferenceScore(request.Content, candidate)
	reasonCodes := append([]string{}, paths...)
	reasonCodes = append(reasonCodes, hardReasons...)
	if len(matchedTerms) > 0 {
		reasonCodes = append(reasonCodes, "lexical_match")
	}
	if len(matchedEntities) > 0 {
		reasonCodes = append(reasonCodes, "entity_match")
	}

	result := ScoredRelevanceCandidate{
		MonitorID: candidate.MonitorID, MonitorConfigVersionID: candidate.MonitorConfigVersionID,
		RecallPaths: paths, MatchedTerms: matchedTerms, MatchedEntities: matchedEntities, ExcludedTerms: excluded,
		Factors:     RelevanceFactors{Lexical: lexicalScore, Entity: entityScore, Title: titleScore, Preference: preference},
		ReasonCodes: uniqueStrings(reasonCodes), HardVeto: hard,
	}
	if hard {
		result.Degraded = true
		result.ScoringVersion = relevanceDegradedVersion
		result.Decision = ingestiondomain.MatchDecisionRejected
		result.ReasonCodes = uniqueStrings(append(result.ReasonCodes, "hard_veto"))
		result.InputHash = relevanceInputHash(request, candidate, result.ScoringVersion, true)
		return result, nil
	}

	var semantic *float64
	if hit.vectorSet {
		value := roundScore(100 * (1 - hit.vector))
		semantic = &value
		result.Factors.Semantic = semantic
		result.EmbeddingProfile = vectorProfile
		result.ScoringVersion = relevanceVersion
		result.RuleScore = roundScore(value*.35 + lexicalScore*.25 + entityScore*.20 + titleScore*.10 + preference*.10)
	} else {
		result.Degraded = true
		result.ScoringVersion = relevanceDegradedVersion
		result.RuleScore = roundScore(lexicalScore*.40 + entityScore*.30 + titleScore*.15 + preference*.15)
		result.ReasonCodes = uniqueStrings(append(result.ReasonCodes, "vector_unavailable"))
	}
	threshold := math.Max(75, candidate.RelevanceThreshold)
	switch {
	case result.RuleScore >= threshold:
		result.Decision = ingestiondomain.MatchDecisionAccepted
	case result.RuleScore < 60:
		result.Decision = ingestiondomain.MatchDecisionRejected
	default:
		result.Decision = ingestiondomain.MatchDecisionReview
	}
	result.InputHash = relevanceInputHash(request, candidate, result.ScoringVersion, result.Degraded)
	return result, nil
}

func recallPaths(hit mergedCandidateHit) []string {
	paths := make([]string, 0, 3)
	if hit.source {
		paths = append(paths, "source")
	}
	if hit.lexical > 0 {
		paths = append(paths, "lexical")
	}
	if hit.vectorSet {
		paths = append(paths, "vector")
	}
	return paths
}

func approvedRules(rules []ingestiondomain.RelevanceRule) []ingestiondomain.RelevanceRule {
	approved := make([]ingestiondomain.RelevanceRule, 0, len(rules))
	for _, rule := range rules {
		if rule.ID <= 0 || strings.TrimSpace(rule.Value) == "" {
			continue
		}
		approved = append(approved, rule)
	}
	sort.Slice(approved, func(left, right int) bool {
		first, second := approved[left], approved[right]
		if first.RuleType != second.RuleType {
			return first.RuleType < second.RuleType
		}
		if normalizedTerm(first.Value) != normalizedTerm(second.Value) {
			return normalizedTerm(first.Value) < normalizedTerm(second.Value)
		}
		if first.Origin != second.Origin {
			return first.Origin < second.Origin
		}
		return first.ID < second.ID
	})
	return approved
}

type weightedTerm struct {
	value  string
	weight float64
}

func splitWeightedRules(rules []ingestiondomain.RelevanceRule) ([]weightedTerm, []weightedTerm) {
	entityByTerm := map[string]weightedTerm{}
	lexicalByTerm := map[string]weightedTerm{}
	for _, rule := range rules {
		term := normalizedTerm(rule.Value)
		if term == "" {
			continue
		}
		weight := effectiveRuleWeight(rule)
		switch rule.RuleType {
		case "entity":
			if prior, exists := entityByTerm[term]; !exists || weight > prior.weight {
				entityByTerm[term] = weightedTerm{value: term, weight: weight}
			}
		case "keyword", "phrase":
			if _, entityExists := entityByTerm[term]; entityExists {
				continue // the same term may contribute only once
			}
			if prior, exists := lexicalByTerm[term]; !exists || weight > prior.weight {
				lexicalByTerm[term] = weightedTerm{value: term, weight: weight}
			}
		}
	}
	lexical, entities := mapTerms(lexicalByTerm), mapTerms(entityByTerm)
	return lexical, entities
}

func mapTerms(terms map[string]weightedTerm) []weightedTerm {
	result := make([]weightedTerm, 0, len(terms))
	for _, term := range terms {
		result = append(result, term)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].value < result[right].value })
	return result
}

func effectiveRuleWeight(rule ingestiondomain.RelevanceRule) float64 {
	weight := rule.Weight / 100
	if weight <= 0 {
		weight = 1
	}
	if weight > 1 {
		weight = 1
	}
	if rule.Origin == "ai" && weight > .60 {
		return .60
	}
	return weight
}

func termScores(content RelevanceContent, terms []weightedTerm) (score float64, matches []string, titleScore float64) {
	if len(terms) == 0 {
		return 0, []string{}, 0
	}
	text, title := normalizedTerm(content.Title+" "+content.Excerpt), normalizedTerm(content.Title)
	var total, matched, titleMatched float64
	for _, term := range terms {
		total += term.weight
		if textContainsTerm(text, term.value) {
			matched += term.weight
			matches = append(matches, term.value)
		}
		if textContainsTerm(title, term.value) {
			titleMatched += term.weight
		}
	}
	if total == 0 {
		return 0, []string{}, 0
	}
	return roundScore(100 * matched / total), matches, roundScore(100 * titleMatched / total)
}

func hardVeto(content RelevanceContent, candidate ingestiondomain.RelevanceCandidate, rules []ingestiondomain.RelevanceRule, lexicalMatch, entityMatch bool) (bool, []string, []string) {
	reasons, excluded := []string{}, []string{}
	if len(candidate.Languages) > 0 && !containsNormalized(candidate.Languages, content.Language) {
		reasons = append(reasons, "language_mismatch")
	}
	if len(candidate.Regions) > 0 && !containsNormalized(candidate.Regions, content.Region) {
		reasons = append(reasons, "region_mismatch")
	}
	text := normalizedTerm(content.Title + " " + content.Excerpt)
	host := contentHost(content.CanonicalURL)
	author := normalizedTerm(content.AuthorExternalID + " " + content.AuthorName)
	hasEntity := false
	for _, rule := range rules {
		value := normalizedTerm(rule.Value)
		switch rule.RuleType {
		case "exclude_keyword":
			if textContainsTerm(text, value) {
				excluded = append(excluded, value)
				reasons = append(reasons, "excluded_term")
			}
		case "language":
			if !ruleMatchesExact(content.Language, value, rule.Operator) {
				reasons = append(reasons, "language_mismatch")
			}
		case "region":
			if !ruleMatchesExact(content.Region, value, rule.Operator) {
				reasons = append(reasons, "region_mismatch")
			}
		case "domain":
			if !ruleMatchesExact(host, value, rule.Operator) {
				reasons = append(reasons, "domain_mismatch")
			}
		case "author":
			if !ruleMatchesExact(author, value, rule.Operator) {
				reasons = append(reasons, "author_mismatch")
			}
		case "entity":
			hasEntity = true
		}
	}
	if hasEntity && lexicalMatch && !entityMatch {
		reasons = append(reasons, "entity_conflict")
	}
	return len(reasons) > 0, uniqueStrings(excluded), uniqueStrings(reasons)
}

func preferenceScore(content RelevanceContent, candidate ingestiondomain.RelevanceCandidate) float64 {
	if (len(candidate.Languages) == 0 || containsNormalized(candidate.Languages, content.Language)) &&
		(len(candidate.Regions) == 0 || containsNormalized(candidate.Regions, content.Region)) {
		return 100
	}
	return 0
}

func ruleMatchesExact(actual, expected, operator string) bool {
	match := normalizedTerm(actual) == normalizedTerm(expected)
	if operator == "not_equals" {
		return !match
	}
	return match
}

func contentHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return normalizedTerm(parsed.Hostname())
}

func containsNormalized(values []string, wanted string) bool {
	wanted = normalizedTerm(wanted)
	for _, value := range values {
		if normalizedTerm(value) == wanted {
			return true
		}
	}
	return false
}

func lexicalLookupTerms(title, excerpt string) []string {
	text := normalizedTerm(title + " " + excerpt)
	words := textWords(text)
	seen := map[string]struct{}{}
	result := make([]string, 0, maximumLexicalTerms)
	add := func(value string) {
		value = normalizedTerm(value)
		if value == "" || len(result) == maximumLexicalTerms {
			return
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	for start := range words {
		for end := start + 1; end <= len(words) && end <= start+16 && len(result) < maximumLexicalTerms; end++ {
			add(strings.Join(words[start:end], " "))
		}
	}
	for _, sequence := range hanSequences(text) {
		runes := []rune(sequence)
		for start := range runes {
			for end := start + 1; end <= len(runes) && end <= start+32 && len(result) < maximumLexicalTerms; end++ {
				add(string(runes[start:end]))
			}
		}
	}
	sort.Strings(result)
	return result
}

func textContainsTerm(text, term string) bool {
	term = normalizedTerm(term)
	if term == "" {
		return false
	}
	if containsHan(term) {
		return strings.Contains(text, term)
	}
	wanted := textWords(term)
	actual := textWords(text)
	if len(wanted) == 0 || len(wanted) > len(actual) {
		return false
	}
	for start := 0; start+len(wanted) <= len(actual); start++ {
		match := true
		for offset := range wanted {
			if actual[start+offset] != wanted[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func normalizedTerm(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(norm.NFC.String(value)), " "))
}

func textWords(value string) []string {
	return strings.FieldsFunc(normalizedTerm(value), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsNumber(character)
	})
}

func containsHan(value string) bool {
	for _, character := range value {
		if unicode.Is(unicode.Han, character) {
			return true
		}
	}
	return false
}

func hanSequences(value string) []string {
	sequences := []string{}
	var current []rune
	for _, character := range value {
		if unicode.Is(unicode.Han, character) {
			current = append(current, character)
			continue
		}
		if len(current) > 0 {
			sequences = append(sequences, string(current))
			current = nil
		}
	}
	if len(current) > 0 {
		sequences = append(sequences, string(current))
	}
	return sequences
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func roundScore(value float64) float64 {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return math.Round(value*100) / 100
}

func relevanceInputHash(request RelevanceScoreRequest, candidate ingestiondomain.RelevanceCandidate, scoringVersion string, degraded bool) string {
	type rule struct {
		Type, Operator, Value, Origin string
		Weight                        float64
	}
	type input struct {
		ContentID                                                       int64
		DedupeKey, Language, Region, Host, AuthorExternalID, AuthorName string
		Title, Excerpt, ConfigHash, ScoringVersion                      string
		Rules                                                           []rule
		Embedding, Review                                               *ModelProfileReference
		Degraded                                                        bool
	}
	rules := approvedRules(candidate.Rules)
	canonicalRules := make([]rule, 0, len(rules))
	for _, value := range rules {
		canonicalRules = append(canonicalRules, rule{Type: value.RuleType, Operator: value.Operator, Value: normalizedTerm(value.Value), Origin: value.Origin, Weight: effectiveRuleWeight(value)})
	}
	payload, _ := json.Marshal(input{
		ContentID: request.Content.ID,
		DedupeKey: request.Content.DedupeKey, Language: normalizedTerm(request.Content.Language), Region: normalizedTerm(request.Content.Region),
		Host: contentHost(request.Content.CanonicalURL), AuthorExternalID: normalizedTerm(request.Content.AuthorExternalID), AuthorName: normalizedTerm(request.Content.AuthorName),
		Title: norm.NFC.String(request.Content.Title), Excerpt: norm.NFC.String(request.Content.Excerpt),
		ConfigHash: candidate.ConfigHash, ScoringVersion: scoringVersion, Rules: canonicalRules,
		Embedding: request.EmbeddingProfile, Review: request.RelevanceReviewProfile, Degraded: degraded,
	})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
