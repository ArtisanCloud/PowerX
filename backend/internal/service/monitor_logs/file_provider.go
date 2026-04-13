package monitorlogs

import (
	"fmt"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
)

type FileProvider struct {
	infoPath  string
	errorPath string
}

func NewFileProvider(cfg *config.Config) *FileProvider {
	p := &FileProvider{}
	if cfg != nil {
		p.infoPath = resolvePath(cfg.LogConfig.File.InfoFilePath)
		p.errorPath = resolvePath(cfg.LogConfig.File.ErrorFilePath)
	}
	if strings.TrimSpace(p.infoPath) == "" {
		p.infoPath = resolvePath("logs/info.log")
	}
	if strings.TrimSpace(p.errorPath) == "" {
		p.errorPath = resolvePath("logs/error.log")
	}
	return p
}

func (p *FileProvider) Driver() Driver { return DriverFile }

func (p *FileProvider) Config() ConfigView {
	return ConfigView{
		Driver: DriverFile,
		Capabilities: Capabilities{
			SupportsLabelQuery:  false,
			SupportsTraceQuery:  true,
			SupportsJobQuery:    true,
			SupportsPolicyQuery: true,
			SupportsGrafanaLink: false,
			HistoryLimited:      false,
			LimitationNote:      "file 驱动不支持 Loki 标签聚合与 Grafana 深链",
		},
	}
}

func (p *FileProvider) Query(req QueryRequest) (QueryResult, error) {
	paths := []string{p.infoPath, p.errorPath}
	lines := make([]string, 0, 2048)
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		part, err := readLinesFromFile(path, 60000)
		if err != nil {
			continue
		}
		lines = append(lines, part...)
	}
	if len(lines) == 0 {
		return QueryResult{Meta: QueryMeta{Driver: DriverFile, Degraded: true, Hint: fmt.Sprintf("未读取到日志文件，请检查路径：%s,%s", p.infoPath, p.errorPath)}}, nil
	}

	entries := make([]Entry, 0, len(lines))
	for i := range lines {
		entries = append(entries, parseLineToEntry(lines[i]))
	}
	filtered := applyFilters(entries, req)
	paged, total := paginate(filtered, req.Page, req.PageSize)
	return QueryResult{Items: paged, Total: total, Meta: QueryMeta{Driver: DriverFile, Degraded: false}}, nil
}
