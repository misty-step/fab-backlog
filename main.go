package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

var (
	org       = flag.String("org", "misty-step", "GitHub org/owner to scan")
	minIssues = flag.Int("min-issues", 5, "minimum issues threshold for health score")
	staleDays = flag.Int("stale-days", 90, "stale threshold in days")
	quiet     = flag.Bool("quiet", false, "suppress info/warn logs (only errors shown)")
	jsonLogs  = flag.Bool("json-logs", false, "emit logs as JSON (default: text)")
)

type output struct {
	GeneratedAt string      `json:"generatedAt"`
	Org         string      `json:"org"`
	Config      config      `json:"config"`
	Repos       []repoScore `json:"repos"`
	Summary     summary     `json:"summary"`
}

type config struct {
	MinIssues int `json:"minIssues"`
	StaleDays int `json:"staleDays"`
}

type repoScore struct {
	Name           string  `json:"name"`
	TotalOpen      int     `json:"totalOpen"`
	StaleCount     int     `json:"staleCount"`
	StalePercent   float64 `json:"stalePercent"`
	UnlabeledCount int     `json:"unlabeledCount"`
	HealthScore    int     `json:"healthScore"`
	Status         string  `json:"status"`
	Error          string  `json:"error,omitempty"`
}

type summary struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Warning  int `json:"warning"`
	Critical int `json:"critical"`
}

type issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Labels    []label   `json:"labels"`
}

type label struct {
	Name string `json:"name"`
}

type repoInfo struct {
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`
	IsArchived    bool   `json:"isArchived"`
}

func main() {
	flag.Parse()

	// Configure slog based on --quiet and --json-logs flags.
	logLevel := slog.LevelInfo
	if *quiet {
		logLevel = slog.LevelError
	}
	var handler slog.Handler
	if *jsonLogs {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	} else {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	}
	slog.SetDefault(slog.New(handler))

	slog.Info("fab-backlog starting", "org", *org, "min_issues", *minIssues, "stale_days", *staleDays)

	out := output{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Org:         *org,
		Config:      config{MinIssues: *minIssues, StaleDays: *staleDays},
		Repos:       []repoScore{},
	}

	slog.Info("scanning repos", "org", *org)
	repos, err := ghListRepos(*org)
	if err != nil {
		slog.Error("failed to list repos", "org", *org, "error", err)
		emitJSON(map[string]any{"ok": false, "error": "failed to list repos: " + err.Error()})
		os.Exit(1)
	}
	slog.Info("repo scan complete", "org", *org, "count", len(repos))

	for _, repo := range repos {
		slog.Info("analysing repo", "repo", repo)
		rs := computeRepoScore(repo, *org, *minIssues, *staleDays)
		if rs.Error != "" {
			slog.Warn("repo analysis error", "repo", repo, "error", rs.Error)
		} else {
			slog.Info("repo analysis complete", "repo", repo, "health_score", rs.HealthScore, "status", rs.Status, "total_open", rs.TotalOpen, "stale_count", rs.StaleCount)
		}
		out.Repos = append(out.Repos, rs)
	}

	sort.Slice(out.Repos, func(i, j int) bool {
		if out.Repos[i].Error != "" && out.Repos[j].Error == "" {
			return false
		}
		if out.Repos[j].Error != "" && out.Repos[i].Error == "" {
			return true
		}
		return out.Repos[i].HealthScore < out.Repos[j].HealthScore
	})

	for _, r := range out.Repos {
		if r.Error != "" {
			continue
		}
		switch r.Status {
		case "healthy":
			out.Summary.Healthy++
		case "warning":
			out.Summary.Warning++
		case "critical":
			out.Summary.Critical++
		}
		out.Summary.Total++
	}

	slog.Info("completed",
		"total", out.Summary.Total,
		"healthy", out.Summary.Healthy,
		"warning", out.Summary.Warning,
		"critical", out.Summary.Critical,
	)

	emitJSON(out)
}

func emitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func ghListRepos(org string) ([]string, error) {
	if strings.TrimSpace(org) == "" {
		return nil, fmt.Errorf("org required")
	}
	args := []string{"repo", "list", org, "--limit", "100", "--json", "name,isArchived"}
	stdout, err := runCmd("gh", args...)
	if err != nil {
		return nil, err
	}
	var repos []repoInfo
	if err := json.Unmarshal(stdout, &repos); err != nil {
		return nil, fmt.Errorf("parse gh repo list json: %w", err)
	}
	var names []string
	for _, r := range repos {
		if !r.IsArchived {
			names = append(names, r.Name)
		}
	}
	return names, nil
}

func ghListIssues(owner, repo string) ([]issue, error) {
	args := []string{"issue", "list", "--repo", owner + "/" + repo, "--state", "open", "--json", "number,title,createdAt,updatedAt,labels", "--limit", "100"}
	stdout, err := runCmd("gh", args...)
	if err != nil {
		return nil, err
	}
	var issues []issue
	if err := json.Unmarshal(stdout, &issues); err != nil {
		return nil, fmt.Errorf("parse gh issue list json: %w", err)
	}
	return issues, nil
}

func computeRepoScore(repoName, org string, minIssues, staleDays int) repoScore {
	score := repoScore{Name: repoName}
	issues, err := ghListIssues(org, repoName)
	if err != nil {
		score.Error = err.Error()
		return score
	}
	score.TotalOpen = len(issues)
	if score.TotalOpen == 0 {
		score.StaleCount, score.StalePercent, score.UnlabeledCount = 0, 0, 0
		score.HealthScore, score.Status = 100, "healthy"
		return score
	}
	staleThreshold := time.Now().AddDate(0, 0, -staleDays)
	for _, issue := range issues {
		if issue.UpdatedAt.Before(staleThreshold) {
			score.StaleCount++
		}
		if len(issue.Labels) == 0 {
			score.UnlabeledCount++
		}
	}
	score.StalePercent = float64(score.StaleCount) / float64(score.TotalOpen) * 100
	unlabeledPercent := float64(score.UnlabeledCount) / float64(score.TotalOpen) * 100
	score.HealthScore = computeHealthScore(score.TotalOpen, score.StalePercent, unlabeledPercent, minIssues)
	if score.HealthScore >= 70 {
		score.Status = "healthy"
	} else if score.HealthScore >= 40 {
		score.Status = "warning"
	} else {
		score.Status = "critical"
	}
	return score
}

func computeHealthScore(totalOpen int, stalePercent, unlabeledPercent float64, minIssues int) int {
	if totalOpen == 0 {
		return 100
	}
	score := 50
	if totalOpen >= minIssues {
		score += 20
	}
	if stalePercent < 30.0 {
		score += 15
	}
	if unlabeledPercent < 20.0 {
		score += 15
	}
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

func IsStale(updatedAt time.Time, staleDays int) bool {
	return updatedAt.Before(time.Now().AddDate(0, 0, -staleDays))
}

func runCmd(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s %s: %s", bin, strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}
