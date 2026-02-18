package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestComputeHealthScore(t *testing.T) {
	tests := []struct {
		totalOpen, minIssues, want int
		stalePercent, unlabeledPercent float64
	}{
		{0, 5, 100, 0, 0},
		{5, 5, 100, 0, 0},
		{10, 5, 85, 50, 0},
		{10, 5, 85, 0, 50},
		{3, 5, 80, 0, 0},
		{20, 5, 70, 80, 80},
		{10, 5, 85, 30, 0},
		{10, 5, 85, 0, 20},
	}
	for i, tt := range tests {
		got := computeHealthScore(tt.totalOpen, tt.stalePercent, tt.unlabeledPercent, tt.minIssues)
		if got != tt.want {
			t.Errorf("case %d: computeHealthScore() = %v, want %v", i, got, tt.want)
		}
	}
}

func TestIsStale(t *testing.T) {
	now := time.Now()
	if IsStale(now, 90) != false {
		t.Error("today should not be stale")
	}
	if IsStale(now.AddDate(0, 0, -89), 90) != false {
		t.Error("89 days ago should not be stale")
	}
	if IsStale(now.AddDate(0, 0, -90), 90) != true {
		t.Error("90 days ago should be stale")
	}
	if IsStale(now.AddDate(-1, 0, 0), 90) != true {
		t.Error("1 year ago should be stale")
	}
}

func TestParseIssues(t *testing.T) {
	data := `[{"number":1,"title":"Bug","labels":[{"name":"bug"}]},{"number":2,"title":"Feature","labels":[]}]`
	var issues []issue
	if err := json.Unmarshal([]byte(data), &issues); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(issues) != 2 || len(issues[0].Labels) != 1 || len(issues[1].Labels) != 0 {
		t.Error("parse mismatch")
	}
}

func TestEdgeCases(t *testing.T) {
	if computeHealthScore(0, 0, 0, 5) != 100 {
		t.Error("zero issues should be 100")
	}
	if computeHealthScore(10, 100, 0, 5) != 85 {
		t.Error("all stale should be 85")
	}
	if computeHealthScore(-5, 0, 0, 5) < 0 {
		t.Error("negative should not produce negative score")
	}
}
