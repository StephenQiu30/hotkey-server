package domain

import (
	"testing"
)

func TestRanking(t *testing.T) {
	score, reasons, err := RankMonitorEvent(RankingInput{RelevanceScore: 90, HeatScore: 80, TrendScore: 20, FreshnessHours: 1})
	if err != nil || score <= 70 || len(reasons) < 3 {
		t.Fatalf("ranking = %v/%#v/%v", score, reasons, err)
	}
}
