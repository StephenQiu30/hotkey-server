package textstructure

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"golang.org/x/text/unicode/norm"
)

// Extractor is a conservative, versioned, local-only structured-key
// extractor. It never calls a model and therefore never discloses body text.
type Extractor struct{}

var _ ingestionapplication.DocumentStructureExtractor = (*Extractor)(nil)

func NewExtractor() *Extractor { return &Extractor{} }

func (*Extractor) ExtractDocumentStructure(_ context.Context, command ingestionapplication.ExtractDocumentStructureCommand) (ingestionapplication.ExtractDocumentStructureResult, error) {
	if err := validateCommand(command); err != nil {
		return ingestionapplication.ExtractDocumentStructureResult{}, err
	}
	searchable := normalizedSearchText(command.Title + "\n" + command.Plaintext)
	entities := boundedEntityKeys(command.Title+"\n"+command.Plaintext, ingestionapplication.MaximumDocumentStructureKeys)
	actions := matchedCanonicalKeys(searchable, actionLexicon, nil)
	locations, regions := matchedLocations(searchable)
	return ingestionapplication.ExtractDocumentStructureResult{
		DocumentVersionID: command.DocumentVersionID,
		ContentSHA256:     command.ContentSHA256,
		ProfileVersion:    ingestionapplication.CanonicalDocumentStructureProfileVersion,
		EntityKeys:        entities,
		ActionKeys:        actions,
		LocationKeys:      locations,
		RegionKeys:        regions,
	}, nil
}

func validateCommand(command ingestionapplication.ExtractDocumentStructureCommand) error {
	if command.DocumentVersionID <= 0 || len(command.ContentSHA256) != 64 || command.Title == "" || len(command.Title) > 16<<10 ||
		command.Plaintext == "" || len(command.Plaintext) > ingestionapplication.MaximumCanonicalSourceBodyBytes ||
		!utf8.ValidString(command.Title) || !utf8.ValidString(command.Plaintext) || strings.ContainsRune(command.Plaintext, '\r') ||
		norm.NFC.String(command.Plaintext) != command.Plaintext || strings.TrimSpace(command.Language) != command.Language || command.Language == "" {
		return sharedrepository.ErrInvalidInput
	}
	for _, r := range command.ContentSHA256 {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return sharedrepository.ErrInvalidInput
		}
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(command.Plaintext))) != command.ContentSHA256 {
		return sharedrepository.ErrConflict
	}
	return nil
}

type lexiconEntry struct {
	canonical string
	aliases   []string
	region    string
}

var actionLexicon = []lexiconEntry{
	{canonical: "acquire", aliases: []string{"acquire", "acquired", "acquires", "acquisition", "收购"}},
	{canonical: "announce", aliases: []string{"announce", "announced", "announces", "announcement", "宣布", "公布"}},
	{canonical: "ban", aliases: []string{"ban", "banned", "bans", "prohibit", "prohibited", "禁止", "禁用"}},
	{canonical: "correct", aliases: []string{"correct", "corrected", "correction", "更正", "勘误"}},
	{canonical: "launch", aliases: []string{"launch", "launched", "launches", "launching", "推出", "上线"}},
	{canonical: "merge", aliases: []string{"merge", "merged", "merger", "合并"}},
	{canonical: "optimize", aliases: []string{"optimize", "optimized", "optimization", "optimise", "optimised", "tuning", "auto-tuning", "调优", "优化"}},
	{canonical: "outage", aliases: []string{"outage", "offline", "service disruption", "故障", "宕机", "中断"}},
	{canonical: "recall", aliases: []string{"recall", "recalled", "召回"}},
	{canonical: "release", aliases: []string{"release", "released", "releases", "发布", "发行"}},
	{canonical: "resign", aliases: []string{"resign", "resigned", "resignation", "辞职", "辞任"}},
	{canonical: "sue", aliases: []string{"sue", "sued", "lawsuit", "litigation", "起诉", "诉讼"}},
	{canonical: "withdraw", aliases: []string{"withdraw", "withdrew", "withdrawn", "撤回", "撤销"}},
}

var locationLexicon = []lexiconEntry{
	{canonical: "beijing", aliases: []string{"beijing", "北京"}, region: "china"},
	{canonical: "china", aliases: []string{"china", "中国", "中华人民共和国"}, region: "china"},
	{canonical: "hong kong", aliases: []string{"hong kong", "香港"}, region: "china"},
	{canonical: "london", aliases: []string{"london", "伦敦"}, region: "uk"},
	{canonical: "san francisco", aliases: []string{"san francisco", "旧金山"}, region: "us"},
	{canonical: "shanghai", aliases: []string{"shanghai", "上海"}, region: "china"},
	{canonical: "shenzhen", aliases: []string{"shenzhen", "深圳"}, region: "china"},
	{canonical: "taiwan", aliases: []string{"taiwan", "台湾"}, region: "china"},
	{canonical: "tokyo", aliases: []string{"tokyo", "东京"}, region: "japan"},
	{canonical: "united kingdom", aliases: []string{"united kingdom", "uk", "u.k.", "英国"}, region: "uk"},
	{canonical: "united states", aliases: []string{"united states", "united states of america", "usa", "u.s.", "美国"}, region: "us"},
}

var entityStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "at": {}, "by": {}, "for": {}, "from": {}, "in": {}, "into": {},
	"of": {}, "on": {}, "or": {}, "the": {}, "to": {}, "with": {},
}

func matchedLocations(searchable string) ([]string, []string) {
	locations := map[string]struct{}{}
	regions := map[string]struct{}{}
	for _, entry := range locationLexicon {
		if matchesAny(searchable, entry.aliases) {
			locations[entry.canonical] = struct{}{}
			if entry.region != "" {
				regions[entry.region] = struct{}{}
			}
		}
	}
	return sortedKeys(locations), sortedKeys(regions)
}

func matchedCanonicalKeys(searchable string, entries []lexiconEntry, excluded map[string]struct{}) []string {
	keys := map[string]struct{}{}
	for _, entry := range entries {
		if _, skip := excluded[entry.canonical]; !skip && matchesAny(searchable, entry.aliases) {
			keys[entry.canonical] = struct{}{}
		}
	}
	return sortedKeys(keys)
}

func matchesAny(searchable string, aliases []string) bool {
	for _, alias := range aliases {
		alias = normalizedSearchText(alias)
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" {
			continue
		}
		if containsHan(trimmed) && strings.Contains(searchable, trimmed) {
			return true
		}
		if strings.Contains(searchable, " "+trimmed+" ") {
			return true
		}
	}
	return false
}

func containsHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func normalizedSearchText(value string) string {
	value = norm.NFC.String(strings.ToLower(value))
	var builder strings.Builder
	builder.Grow(len(value) + 2)
	builder.WriteByte(' ')
	lastSpace := true
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.Han, r) || r == '-' || r == '+' || r == '#' {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	if !lastSpace {
		builder.WriteByte(' ')
	}
	return builder.String()
}

func boundedEntityKeys(value string, limit int) []string {
	keys := map[string]struct{}{}
	latinTokens, hanRuns := entityTokens(value)
	for index, token := range latinTokens {
		if _, stop := entityStopWords[token]; !stop && meaningfulEntityToken(token) {
			keys[token] = struct{}{}
		}
		for width := 2; width <= 3 && index+width <= len(latinTokens); width++ {
			phrase := strings.Join(latinTokens[index:index+width], " ")
			if len(phrase) <= 160 {
				keys[phrase] = struct{}{}
			}
		}
	}
	for _, run := range hanRuns {
		runes := []rune(run)
		if len(runes) >= 2 && len(runes) <= 32 {
			keys[run] = struct{}{}
		}
		for width := 2; width <= 6; width++ {
			for start := 0; start+width <= len(runes); start++ {
				keys[string(runes[start:start+width])] = struct{}{}
			}
		}
	}
	result := sortedKeys(keys)
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func meaningfulEntityToken(value string) bool {
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			return true
		}
	}
	return false
}

func entityTokens(value string) ([]string, []string) {
	value = norm.NFC.String(strings.ToLower(value))
	latin := []string{}
	han := []string{}
	var token strings.Builder
	var hanRun strings.Builder
	flushToken := func() {
		if token.Len() > 0 {
			latin = append(latin, token.String())
			token.Reset()
		}
	}
	flushHan := func() {
		if hanRun.Len() > 0 {
			han = append(han, hanRun.String())
			hanRun.Reset()
		}
	}
	for _, r := range value {
		switch {
		case unicode.Is(unicode.Han, r):
			flushToken()
			hanRun.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '+' || r == '#':
			flushHan()
			token.WriteRune(r)
		default:
			flushToken()
			flushHan()
		}
	}
	flushToken()
	flushHan()
	return latin, han
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
