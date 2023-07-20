package gen

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type ServerInfo struct {
	Group  string
	Prefix string
	Title  string
	Desc   string
}

type Route struct {
	Doc    string
	Method string
	Path   string
}

func GenAPICsv(files []*os.File) {
	apiFile, err := os.OpenFile("api.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	groupFile, err := os.OpenFile("group.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	apiWriter := csv.NewWriter(apiFile)
	groupWriter := csv.NewWriter(groupFile)

	pattern := `(?s)service\s+.*\s*{.*}`
	re := regexp.MustCompile(pattern)

	for _, file := range files {
		log.Printf("正在处理文件: %s", file.Name())

		// 排除 powerx 主文件，主文件不分组，内部也不应该有api
		if filepath.Base(file.Name()) == "powerx.api" {
			continue
		}

		apiText, err := io.ReadAll(file)
		if err != nil {
			log.Fatalln(err)
		}

		if !re.MatchString(string(apiText)) {
			log.Printf("文件 %s 不是一个有效的 .api 文件, 跳过", file.Name())
			continue
		}

		serverInfo, routes := parseApiText(string(apiText))
		groupWriter.Write([]string{serverInfo.Group, serverInfo.Prefix, serverInfo.Title, serverInfo.Desc})
		for _, route := range routes {
			apiWriter.Write([]string{serverInfo.Group, path.Join(serverInfo.Prefix, route.Path), route.Method, route.Doc})
		}
	}
	apiWriter.Flush()
	groupWriter.Flush()
}

func parseApiText(text string) (ServerInfo, []Route) {
	serverInfo := extractServerInfo(text)
	routes := extractRoutes(text)
	return serverInfo, routes
}

func extractServerInfo(text string) ServerInfo {
	groupRegex := regexp.MustCompile(`group:\s+(.+)`)
	prefixRegex := regexp.MustCompile(`prefix:\s+(.+)`)

	group := strings.TrimSpace(groupRegex.FindStringSubmatch(text)[1])
	prefix := strings.TrimSpace(prefixRegex.FindStringSubmatch(text)[1])

	titlePattern := regexp.MustCompile(`title:\s*"(.*?)"`)
	descPattern := regexp.MustCompile(`desc:\s*"(.*?)"`)

	titleMatches := titlePattern.FindStringSubmatch(text)
	descMatches := descPattern.FindStringSubmatch(text)

	title := "待命名分组"
	desc := "待描述"

	if len(titleMatches) > 1 {
		title = titleMatches[1]
	}

	if len(descMatches) > 1 {
		desc = descMatches[1]
	}

	return ServerInfo{
		Group:  group,
		Prefix: prefix,
		Title:  title,
		Desc:   desc,
	}
}

func extractRoutes(text string) []Route {
	docRegex := regexp.MustCompile(`@doc\s+"(.+?)"`)
	methodRegex := regexp.MustCompile(`(?m)(post|get|patch|put|delete)\s`)
	pathRegex := regexp.MustCompile(`(?m)(post|get|patch|put|delete)\s+(.+?)\s`)

	docs := docRegex.FindAllStringSubmatch(text, -1)
	methods := methodRegex.FindAllStringSubmatch(text, -1)
	paths := pathRegex.FindAllStringSubmatch(text, -1)

	var routes []Route
	for i := range docs {
		routes = append(routes, Route{
			Doc:    strings.TrimSpace(docs[i][1]),
			Method: strings.TrimSpace(methods[i][1]),
			Path:   strings.TrimSpace(paths[i][2]),
		})
	}

	return routes
}

func FindAPIFiles(dir string) ([]*os.File, error) {
	var files []*os.File
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".api" {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			files = append(files, file)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return files, nil
}
