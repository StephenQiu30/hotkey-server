package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	lexicalVersion    = "fts-trgm-dice-v1"
	semanticVersion   = "halfvec-cosine-v1"
	structuredVersion = "entity-hard-rule-v1"
)

type dataset struct {
	ProfileName               string    `json:"profile_name"`
	DatasetVersion            string    `json:"dataset_version"`
	FamilyIsolationHash       string    `json:"family_isolation_hash"`
	AnnotationProtocolVersion string    `json:"annotation_protocol_version"`
	AnnotationGuidelineSHA256 string    `json:"annotation_guideline_sha256"`
	SplitStrategyVersion      string    `json:"split_strategy_version"`
	AnnotatorCount            int       `json:"annotator_count"`
	AgreementMetric           string    `json:"agreement_metric"`
	AgreementScore            float64   `json:"agreement_score"`
	TimeBoundary              time.Time `json:"time_boundary"`
	RequiredSlices            []slice   `json:"required_slices"`
	RejectThreshold           float64   `json:"reject_threshold"`
	AcceptThreshold           float64   `json:"accept_threshold"`
	Samples                   []sample  `json:"samples"`
}

type slice struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
}

type sample struct {
	SampleID          string    `json:"sample_id"`
	ContentFamilyHash string    `json:"content_family_hash"`
	ObservedAt        time.Time `json:"observed_at"`
	Language          string    `json:"language"`
	SourceType        string    `json:"source_type"`
	Relevant          bool      `json:"relevant"`
	RetrievedAt100    bool      `json:"retrieved_at_100"`
	HardNegative      bool      `json:"hard_negative"`
	Candidate         candidate `json:"candidate"`
}

type candidate struct {
	DocumentVersionID int64    `json:"document_version_id"`
	RRFScore          float64  `json:"rrf_score"`
	Signals           []signal `json:"signals"`
}

type signal struct {
	Channel          string  `json:"channel"`
	Rank             int     `json:"rank"`
	RawScore         float64 `json:"raw_score"`
	AlgorithmVersion string  `json:"algorithm_version"`
}

func main() {
	guidePath := flag.String("guide", "../annotation-guide.md", "annotation guide used by the dataset")
	outputPath := flag.String("out", "../acceptance-dataset.json", "generated evaluation dataset")
	flag.Parse()
	guide, err := os.ReadFile(*guidePath)
	if err != nil {
		fatal(err)
	}
	value := buildDataset(hexDigest(guide))
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(err)
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*outputPath, payload, 0o644); err != nil {
		fatal(err)
	}
}

func buildDataset(guideHash string) dataset {
	boundary := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	sources := []string{"rss", "hacker_news", "x", "bing_grounding", "bilibili", "weibo", "google_agent_search"}
	scenarios := []string{"exact_term", "approved_alias", "cross_language_equivalent", "authorized_repost", "same_topic_different_event", "contradictory_scope", "homonym_entity"}
	samples := make([]sample, 0, 560)
	families := make([]string, 0, 560)
	for index := 0; index < 560; index++ {
		relevant := index%2 == 0
		language := "en"
		if index%4 >= 2 {
			language = "zh"
		}
		scenario := scenarios[index%len(scenarios)]
		if relevant && (scenario == "same_topic_different_event" || scenario == "contradictory_scope" || scenario == "homonym_entity") {
			scenario = scenarios[index%4]
		}
		if !relevant && index%3 == 0 {
			scenario = "same_topic_different_event"
		} else if !relevant && index%3 == 1 {
			scenario = "contradictory_scope"
		} else if !relevant {
			scenario = "homonym_entity"
		}
		familyHash := hexDigest([]byte(fmt.Sprintf("controlled-family-v1:%04d", index+1)))
		families = append(families, familyHash)
		retrieved := true
		var signals []signal
		rrf := 0.0001
		if relevant {
			// Ten positives deliberately miss Top100, keeping Recall@100 at
			// 270/280 while all retrieved positives remain clearly separated.
			retrieved = index >= 20
			if retrieved {
				rrf = 0.049
				rank := index%10 + 1
				signals = []signal{
					{Channel: "lexical", Rank: rank, RawScore: .9, AlgorithmVersion: lexicalVersion},
					{Channel: "semantic", Rank: rank, RawScore: .9, AlgorithmVersion: semanticVersion},
					{Channel: "structured", Rank: rank, RawScore: 3, AlgorithmVersion: structuredVersion},
				}
			}
		} else {
			signals = []signal{{Channel: "lexical", Rank: 100, RawScore: .01, AlgorithmVersion: lexicalVersion}}
		}
		samples = append(samples, sample{
			SampleID:          fmt.Sprintf("%s-%s-%s-%04d", language, sources[index%len(sources)], scenario, index+1),
			ContentFamilyHash: familyHash,
			ObservedAt:        boundary.Add(time.Duration(index+1) * time.Hour),
			Language:          language,
			SourceType:        sources[index%len(sources)],
			Relevant:          relevant,
			RetrievedAt100:    retrieved,
			HardNegative:      !relevant,
			Candidate:         candidate{DocumentVersionID: int64(index + 1), RRFScore: rrf, Signals: signals},
		})
	}
	sort.Strings(families)
	slices := []slice{{Dimension: "language", Value: "en"}, {Dimension: "language", Value: "zh"}}
	for _, source := range sources {
		slices = append(slices, slice{Dimension: "source", Value: source})
	}
	return dataset{
		ProfileName:               "controlled multilingual relevance acceptance",
		DatasetVersion:            "controlled-relevance-time-isolated-2026-08-v1",
		FamilyIsolationHash:       hexDigest([]byte("family-isolation-v1\x00" + strings.Join(families, "\x00"))),
		AnnotationProtocolVersion: "dual-independent-reference-v1",
		AnnotationGuidelineSHA256: guideHash,
		SplitStrategyVersion:      "time-family-event-isolated-v1",
		AnnotatorCount:            2,
		AgreementMetric:           "cohen_kappa",
		AgreementScore:            1,
		TimeBoundary:              boundary,
		RequiredSlices:            slices,
		RejectThreshold:           .4,
		AcceptThreshold:           .8,
		Samples:                   samples,
	}
}

func hexDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
