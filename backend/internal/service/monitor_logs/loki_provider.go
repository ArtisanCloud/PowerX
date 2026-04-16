package monitorlogs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
)

type LokiProvider struct {
	baseURL string
	jobName string
	client  *http.Client
}

func NewLokiProvider(cfg *config.Config) *LokiProvider {
	p := &LokiProvider{
		client: &http.Client{Timeout: 10 * time.Second},
	}
	if cfg != nil {
		p.baseURL = strings.TrimSpace(cfg.LogConfig.Loki.URL)
		p.jobName = strings.TrimSpace(cfg.LogConfig.Loki.JobName)
	}
	if p.jobName == "" {
		p.jobName = "powerx"
	}
	return p
}

func (p *LokiProvider) Driver() Driver { return DriverLoki }

func (p *LokiProvider) Config() ConfigView {
	return ConfigView{
		Driver:         DriverLoki,
		GrafanaBaseURL: grafanaBaseFromLokiURL(p.baseURL),
		Capabilities: Capabilities{
			SupportsLabelQuery:  true,
			SupportsTraceQuery:  true,
			SupportsJobQuery:    true,
			SupportsPolicyQuery: true,
			SupportsGrafanaLink: true,
			HistoryLimited:      false,
		},
	}
}

func (p *LokiProvider) Query(req QueryRequest) (QueryResult, error) {
	if strings.TrimSpace(p.baseURL) == "" {
		return QueryResult{Meta: QueryMeta{Driver: DriverLoki, Degraded: true, Hint: "loki.url 未配置，已降级到无结果"}}, nil
	}
	end := time.Now().UTC()
	if req.To != nil {
		end = req.To.UTC()
	}
	start := end.Add(-15 * time.Minute)
	if req.From != nil {
		start = req.From.UTC()
	}
	if start.After(end) {
		start, end = end, start
	}

	limit := req.Page * req.PageSize
	if limit < 200 {
		limit = 200
	}
	if limit > 5000 {
		limit = 5000
	}
	query := p.buildQuery(req)
	entries, err := p.fetchRange(query, start, end, limit)
	if err != nil {
		return QueryResult{}, err
	}
	filtered := applyFilters(entries, req)
	paged, total := paginate(filtered, req.Page, req.PageSize)
	return QueryResult{
		Items: paged,
		Total: total,
		Meta: QueryMeta{
			Driver:   DriverLoki,
			Degraded: false,
			Grafana:  buildGrafanaExploreURL(grafanaBaseFromLokiURL(p.baseURL), query, start, end),
		},
	}, nil
}

func (p *LokiProvider) buildQuery(req QueryRequest) string {
	selectors := []string{fmt.Sprintf("job=\"%s\"", escapeLokiString(p.jobName))}
	base := "{" + strings.Join(selectors, ",") + "}"
	filters := make([]string, 0, 4)
	if v := strings.TrimSpace(req.TraceID); v != "" {
		filters = append(filters, fmt.Sprintf("|= \"%s\"", escapeLokiString(v)))
	}
	if req.JobID > 0 {
		filters = append(filters, fmt.Sprintf("|= \"job_id\\\": %d\"", req.JobID))
	}
	if req.PolicyID > 0 {
		filters = append(filters, fmt.Sprintf("|= \"policy_id\\\": %d\"", req.PolicyID))
	}
	if v := strings.TrimSpace(req.Keyword); v != "" {
		filters = append(filters, fmt.Sprintf("|= \"%s\"", escapeLokiString(v)))
	}
	if len(filters) == 0 {
		return base
	}
	return base + " " + strings.Join(filters, " ")
}

func (p *LokiProvider) fetchRange(query string, start, end time.Time, limit int) ([]Entry, error) {
	u, err := url.Parse(strings.TrimRight(p.baseURL, "/") + "/loki/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("limit", strconv.Itoa(limit))
	q.Set("direction", "BACKWARD")
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("loki query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed lokiQueryResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("loki query status=%s", parsed.Status)
	}
	entries := make([]Entry, 0, 256)
	for i := range parsed.Data.Result {
		stream := parsed.Data.Result[i]
		for j := range stream.Values {
			pair := stream.Values[j]
			if len(pair) < 2 {
				continue
			}
			ts, _ := strconv.ParseInt(strings.TrimSpace(pair[0]), 10, 64)
			line := pair[1]
			e := parseLineToEntry(line)
			if ts > 0 {
				e.Timestamp = time.Unix(0, ts).UTC()
			}
			entries = append(entries, e)
		}
	}
	return entries, nil
}

type lokiQueryResp struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Values [][]string `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func escapeLokiString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func grafanaBaseFromLokiURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme == "" || u.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}

func buildGrafanaExploreURL(base, query string, from, to time.Time) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	u.Path = "/explore"
	qv := u.Query()
	qv.Set("left", fmt.Sprintf("{\"queries\":[{\"expr\":%q}],\"range\":{\"from\":\"%s\",\"to\":\"%s\"}}", query, from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano)))
	u.RawQuery = qv.Encode()
	return u.String()
}
