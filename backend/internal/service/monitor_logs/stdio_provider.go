package monitorlogs

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/runtimebuffer"
)

type StdioProvider struct {
	fallback *FileProvider
}

func NewStdioProvider(cfg *config.Config) *StdioProvider {
	return &StdioProvider{fallback: NewFileProvider(cfg)}
}

func (p *StdioProvider) Driver() Driver { return DriverStdio }

func (p *StdioProvider) Config() ConfigView {
	return ConfigView{
		Driver: DriverStdio,
		Capabilities: Capabilities{
			SupportsLabelQuery:  false,
			SupportsTraceQuery:  true,
			SupportsJobQuery:    true,
			SupportsPolicyQuery: true,
			SupportsGrafanaLink: false,
			HistoryLimited:      true,
			LimitationNote:      "stdio 驱动仅支持最近窗口查询，历史范围取决于进程输出缓冲与本地落盘情况",
		},
	}
}

func (p *StdioProvider) Query(req QueryRequest) (QueryResult, error) {
	b := runtimebuffer.Snapshot(512 * 1024)
	if len(b) > 0 {
		lines := strings.Split(string(bytes.TrimSpace(b)), "\n")
		entries := make([]Entry, 0, len(lines))
		for i := range lines {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			entries = append(entries, parseLineToEntry(line))
		}
		filtered := applyFilters(entries, req)
		paged, total := paginate(filtered, req.Page, req.PageSize)
		return QueryResult{
			Items: paged,
			Total: total,
			Meta:  QueryMeta{Driver: DriverStdio, Degraded: true, Hint: "当前为 stdio 最近窗口日志（进程内 ring buffer）"},
		}, nil
	}

	if p.fallback == nil {
		return QueryResult{Meta: QueryMeta{Driver: DriverStdio, Degraded: true, Hint: "stdio ring buffer unavailable"}}, nil
	}
	res, err := p.fallback.Query(req)
	if err != nil {
		return QueryResult{}, err
	}
	res.Meta.Driver = DriverStdio
	if strings.TrimSpace(res.Meta.Hint) == "" {
		res.Meta.Hint = "当前为 stdio 模式，返回结果可能仅包含最近窗口日志"
	}
	res.Meta.Degraded = true
	if len(res.Items) == 0 && strings.TrimSpace(res.Meta.Hint) == "" {
		res.Meta.Hint = fmt.Sprintf("未检索到 stdio 最近窗口日志")
	}
	return res, nil
}
