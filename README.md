# fab-backlog

A Go CLI tool that assesses backlog health across GitHub repositories. It analyzes open issues for staleness, labeling gaps, and scope quality to help teams maintain a healthy project backlog.

## What It Does

**fab-backlog** scans all non-archived repositories in a GitHub organization and produces a health report based on:

- **Staleness**: Percentage of issues not updated in the configured number of days (default: 90)
- **Labeling**: Percentage of issues without any labels
- **Volume**: Whether the repo meets the minimum issue threshold

Each repo receives a **health score** (0-100) and a status:
- **Healthy** (≥70): Good backlog hygiene
- **Warning** (40-69): Needs attention
- **Critical** (<40): Requires immediate cleanup

## Installation

```bash
go install github.com/misty-step/fab-backlog@latest
```

**Prerequisites:**
- Go 1.25+
- [GitHub CLI (`gh`)](https://cli.github.com/) installed and authenticated

Authenticate with GitHub:
```bash
gh auth login
```

## Usage

### Basic Scan

Scan all repos in the `misty-step` organization (default):

```bash
fab-backlog
```

Scan a specific organization:

```bash
fab-backlog -org my-org
```

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-org` | `misty-step` | GitHub organization/owner to scan |
| `-min-issues` | `5` | Minimum open issues required for full health score |
| `-stale-days` | `90` | Days after which an issue is considered stale |

### Examples

```bash
# Scan with custom stale threshold (60 days)
fab-backlog -stale-days 60

# Scan org with higher volume expectation
fab-backlog -org my-team -min-issues 10

# Combine flags
fab-backlog -org my-org -min-issues 10 -stale-days 60
```

## Output Format

The tool outputs JSON to stdout. Example output:

```json
{
  "generatedAt": "2025-02-18T13:44:00Z",
  "org": "misty-step",
  "config": {
    "minIssues": 5,
    "staleDays": 90
  },
  "repos": [
    {
      "name": "some-repo",
      "totalOpen": 23,
      "staleCount": 2,
      "stalePercent": 8.7,
      "unlabeledCount": 5,
      "healthScore": 85,
      "status": "healthy"
    },
    {
      "name": "neglected-repo",
      "totalOpen": 45,
      "staleCount": 38,
      "stalePercent": 84.4,
      "unlabeledCount": 30,
      "healthScore": 35,
      "status": "critical"
    }
  ],
  "summary": {
    "total": 15,
    "healthy": 10,
    "warning": 3,
    "critical": 2
  }
}
```

### Health Score Calculation

```
Base score: 50
+20 if totalOpen >= minIssues
+15 if stalePercent < 30%
+15 if unlabeledPercent < 20%
Final score capped at 0-100
```

### Status Thresholds

| Status | Score Range |
|--------|-------------|
| Healthy | ≥ 70 |
| Warning | 40-69 |
| Critical | < 40 |

## Configuration

No configuration file required. All settings are passed via CLI flags:

- `-org`: Target GitHub organization (uses `gh auth` credentials)
- `-min-issues`: Adjust based on your team's typical repo size
- `-stale-days`: Tune based on your release cycle (90 days = ~3 months)

## Integration

fab-backlog is designed for factory automation workflows:

### CI/CD Integration

Add to your daily/weekly cron job to track backlog health over time:

```bash
# In cron or automation script
fab-backlog -org my-org > backlog-health-$(date +%Y-%m-%d).json
```

### Pre-commit Checks

Run before releases to ensure backlog is well-maintained:

```bash
fab-backlog -org my-org | jq -r '.repos[] | select(.status == "critical") | .name'
```

### GitHub Actions

```yaml
name: Backlog Health Check
on:
  schedule:
    - cron: '0 9 * * 1'  # Weekly on Monday
jobs:
  health:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go install github.com/misty-step/fab-backlog@latest
      - run: fab-backlog -org ${{ github.repository_owner }}
      - name: Post to Slack on critical
        if: success()
        run: |
          fab-backlog -org ${{ github.repository_owner }} \
          | jq -r '.repos[] | select(.status=="critical") | "\(.name): \(.healthScore)"' \
          | slack/notify.sh
```

## Contributing

Standard Go workflow:

```bash
# Clone the repo
git clone https://github.com/misty-step/fab-backlog.git
cd fab-backlog

# Create a feature branch
git checkout -b your-feature

# Run tests
go test -v ./...

# Run the tool locally
go run . -org your-test-org

# Commit and push
git add .
git commit -m "feat: your feature"
git push origin your-feature
```

### Development Notes

- The tool uses the `gh` CLI for all GitHub API interactions
- Output is JSON for easy parsing in automation pipelines
- Repos are sorted by health score (worst first) in output
- Archived repos are automatically excluded from scans

## License

MIT