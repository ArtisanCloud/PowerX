package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type occurrence struct {
	file  string
	count int
	hits  []hit
}

type hit struct {
	line int
	text string
	mode string
}

var (
	rootDir     string
	excludeDirs string
	maxFileSize int64
	patterns    string
	outputFile  string
)

func init() {
	flag.StringVar(&rootDir, "root", ".", "扫描根目录")
	flag.StringVar(&excludeDirs, "exclude-dirs", ".git,node_modules,tmp,dist,vendor,build", "需要排除的目录，逗号分隔")
	flag.Int64Var(&maxFileSize, "max-bytes", 2*1024*1024, "单个文件扫描大小上限")
	flag.StringVar(&patterns, "patterns", "(?i)tenant[_-]?id", "正则表达式，匹配 legacy tenant_id 用法")
	flag.StringVar(&outputFile, "output", "", "输出 Markdown 文件路径，默认写到 stdout")
}

func main() {
	flag.Parse()
	matcher, err := regexp.Compile(patterns)
	if err != nil {
		logger.ErrorF(context.Background(), "invalid regex: %v", err)
		os.Exit(1)
	}

	excludes := buildExcludeSet(excludeDirs)
	occs := make([]occurrence, 0)

	err = filepath.WalkDir(rootDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			rel = path
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name(), excludes) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipFile(path, d, maxFileSize) {
			return nil
		}
		hits, err := scanFile(path, matcher)
		if err != nil {
			return err
		}
		if len(hits) > 0 {
			occs = append(occs, occurrence{file: rel, count: len(hits), hits: hits})
		}
		return nil
	})
	if err != nil {
		logger.ErrorF(context.Background(), "walk error: %v", err)
		os.Exit(1)
	}

	sort.Slice(occs, func(i, j int) bool {
		if occs[i].count == occs[j].count {
			return occs[i].file < occs[j].file
		}
		return occs[i].count > occs[j].count
	})

	builder := &strings.Builder{}
	writeReport(builder, occs)

	if outputFile == "" {
		logger.InfoF(context.Background(), "%s", builder.String())
		return
	}
	if err := os.WriteFile(outputFile, []byte(builder.String()), 0o644); err != nil {
		logger.ErrorF(context.Background(), "write output failed: %v", err)
		os.Exit(1)
	}
}

func buildExcludeSet(list string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, part := range strings.Split(list, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	return set
}

func shouldSkipDir(name string, excludes map[string]struct{}) bool {
	_, skip := excludes[name]
	return skip
}

func shouldSkipFile(path string, d os.DirEntry, maxBytes int64) bool {
	if d.Type()&os.ModeSymlink != 0 {
		return true
	}
	info, err := d.Info()
	if err != nil {
		return true
	}
	if info.Size() > maxBytes {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return true
	}
	if isBinary(buf[:n]) {
		return true
	}
	return false
}

func isBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func scanFile(path string, matcher *regexp.Regexp) ([]hit, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	hits := make([]hit, 0)
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		if matcher.MatchString(text) {
			hits = append(hits, hit{
				line: line,
				text: strings.TrimSpace(text),
				mode: detectSQLMode(text),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return hits, nil
}

func writeReport(w io.Writer, occs []occurrence) {
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Fprintf(w, "# Tenant ID Usage Report\n\n")
	fmt.Fprintf(w, "- 生成时间：%s\n", timestamp)
	fmt.Fprintf(w, "- 扫描路径：%s\n\n", rootDir)
	if len(occs) == 0 {
		fmt.Fprintln(w, "✅ 未发现 `tenant_id` 匹配项。")
		return
	}
	fmt.Fprintf(w, "| 序号 | 文件 | 命中数 | SQL 模式 |\n| --- | --- | --- | --- |\n")
	for idx, occ := range occs {
		fmt.Fprintf(
			w,
			"| %d | %s | %d | %s |\n",
			idx+1,
			escapePipe(occ.file),
			occ.count,
			strings.Join(uniqueModes(occ.hits), ", "),
		)
	}
	fmt.Fprintln(w)
	for _, occ := range occs {
		fmt.Fprintf(w, "## %s (%d)\n", occ.file, occ.count)
		for _, h := range occ.hits {
			snippet := h.text
			if len(snippet) > 160 {
				snippet = snippet[:160] + "..."
			}
			fmt.Fprintf(w, "- [%s] L%d: `%s`\n", modeDisplay(h.mode), h.line, snippet)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "> 运行命令：`go run ./tools/tenant-uuid-scan -root %s`\n", rootDir)
}

func escapePipe(input string) string {
	return strings.ReplaceAll(input, "|", "\\|")
}

const (
	modeIsNotDistinct = "is_not_distinct"
	modeInClause      = "in_clause"
	modeComparison    = "comparison"
	modeEquals        = "equals_placeholder"
	modeGeneric       = "generic"
)

func detectSQLMode(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "is not distinct from"):
		return modeIsNotDistinct
	case strings.Contains(lower, " in ") && strings.Contains(lower, "tenant"):
		return modeInClause
	case strings.Contains(lower, "<>") || strings.Contains(lower, "!="):
		return modeComparison
	case strings.Contains(lower, "= ?") || strings.Contains(lower, "=?"):
		return modeEquals
	default:
		return modeGeneric
	}
}

func modeDisplay(mode string) string {
	switch mode {
	case modeIsNotDistinct:
		return "IS NOT DISTINCT"
	case modeInClause:
		return "IN (...)"
	case modeComparison:
		return "<>/!="
	case modeEquals:
		return "= ?"
	default:
		return "generic"
	}
}

func uniqueModes(hits []hit) []string {
	seen := make(map[string]struct{})
	order := make([]string, 0, len(hits))
	for _, h := range hits {
		label := modeDisplay(h.mode)
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		order = append(order, label)
	}
	return order
}
