package plugin

import (
	"bytes"
	"fmt"
	"github.com/zeromicro/go-zero/tools/goctl/api/parser"
	"github.com/zeromicro/go-zero/tools/goctl/api/spec"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

const (
	apiDir          = "api"
	packageName     = "powerx"
	typesImportPath = "PluginTemplate/pkg/powerx/powerxtypes"
)

type StringMap = map[string]string

type TemplateData struct {
	PackageName      string
	TypesImportPath  string
	GroupName        string
	Routes           []spec.Route
	PathParamsMap    map[string]StringMap
	HasTypesImport   bool
	HasFmtImport     bool
	HasNetHttpImport bool
}

func parseApiFile(filePath string) (*spec.ApiSpec, error) {
	api, err := parser.Parse(filePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing .api file: %w", err)
	}
	return api, nil
}

func GenerateClientCode(api *spec.ApiSpec, targetDir string) error {
	if err := createTargetDir(targetDir); err != nil {
		return err
	}

	tmpl, err := parseClientTemplate()
	if err != nil {
		return err
	}

	var apiModules []string
	for _, group := range api.Service.Groups {
		templateData, err := prepareTemplateData(group)
		if err != nil {
			return err
		}

		if err := generateGroupClientCode(tmpl, targetDir, templateData); err != nil {
			return err
		}
		if templateData.GroupName != "" {
			apiModules = append(apiModules, templateData.GroupName)
		}
	}

	tmpl, err = parsePowerXTemple()
	if err != nil {
		return err
	}
	if err := generatePowerXCode(tmpl, targetDir, apiModules); err != nil {
		return err
	}

	return nil
}

func createTargetDir(targetDir string) error {
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		err = os.MkdirAll(targetDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("error creating target dir: %w", err)
		}
	}
	return nil
}

func parseClientTemplate() (*template.Template, error) {
	tmplContent, err := ioutil.ReadFile(GetResourcePath("client.tpl"))
	if err != nil {
		return nil, fmt.Errorf("error reading client.tpl: %w", err)
	}

	funcMap := template.FuncMap{
		"CapFirst":   CapitalizeFirstLetter,
		"ToUpper":    ToUpperCase,
		"FormatPath": FormatPath,
	}

	return template.New("clientCode").Funcs(funcMap).Parse(string(tmplContent))
}

func parsePowerXTemple() (*template.Template, error) {
	tmplContent, err := ioutil.ReadFile(GetResourcePath("powerx.tpl"))
	if err != nil {
		return nil, fmt.Errorf("error reading powerx.tpl: %w", err)
	}
	return template.New("powerxCode").Parse(string(tmplContent))
}

func prepareTemplateData(group spec.Group) (TemplateData, error) {
	groupName := formatGroupName(group.Annotation.Properties["group"])

	templateData := TemplateData{
		PackageName:      packageName,
		TypesImportPath:  typesImportPath,
		GroupName:        groupName,
		Routes:           group.Routes,
		PathParamsMap:    make(map[string]StringMap),
		HasTypesImport:   false,
		HasFmtImport:     false,
		HasNetHttpImport: false,
	}

	if len(group.Routes) == 0 {
		return templateData, nil
	}

	prefix := group.GetAnnotation("prefix")
	for i, route := range templateData.Routes {
		templateData.Routes[i].Path = path.Join(prefix, route.Path)

		if route.RequestType == nil {
			continue
		}

		switch req := route.RequestType.(type) {
		case spec.DefineStruct:
			members := req.GetTagMembers("path")
			if len(members) == 0 {
				continue
			}
			for _, member := range members {
				if member.IsTagMember("path") && member.Name != "" {
					if templateData.PathParamsMap[route.Handler] == nil {
						templateData.PathParamsMap[route.Handler] = make(StringMap)
					}
					tagParts := strings.Split(member.Tag, ":")
					if len(tagParts) < 2 {
						continue
					}
					pathKey := strings.Trim(tagParts[1], "`\"")
					templateData.PathParamsMap[route.Handler][pathKey] = member.Name
				}
			}
		}
	}

	if len(templateData.PathParamsMap) != 0 {
		templateData.HasFmtImport = true
	}
	templateData.HasNetHttpImport = true

	for _, route := range templateData.Routes {
		if route.RequestTypeName() != "" {
			templateData.HasTypesImport = true
		}
	}

	return templateData, nil
}

func generateGroupClientCode(tmpl *template.Template, targetDir string, templateData TemplateData) error {
	// 如果 groupName 为空，则不生成 client 代码
	if templateData.GroupName == "" {
		return nil
	}
	// 如果 routes 为空，则不生成 client 代码
	if len(templateData.Routes) == 0 {
		return nil
	}
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, templateData)
	if err != nil {
		return fmt.Errorf("error executing client template: %w", err)
	}

	err = ioutil.WriteFile(filepath.Join(targetDir, strings.ToLower(templateData.GroupName)+"client.go"), buf.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("error writing client code: %w", err)
	}

	return nil
}

func generatePowerXCode(tmpl *template.Template, targetDir string, apiModules []string) error {
	// 如果 groupName 为空，则不生成 client 代码
	if len(apiModules) == 0 {
		return nil
	}
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, apiModules)
	if err != nil {
		return fmt.Errorf("error executing client template: %w", err)
	}

	err = ioutil.WriteFile(filepath.Join(targetDir, "powerx.go"), buf.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("error writing client code: %w", err)
	}

	return nil
}

func GetResourcePath(file string) string {
	_, currentFilePath, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(currentFilePath)
	return filepath.Join(currentDir, file)
}

func CapitalizeFirstLetter(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func ToUpperCase(s string) string {
	return strings.ToUpper(s)
}

func FormatPath(path string, handler string, pathParamsMap map[string]StringMap) string {
	var values []string
	if params, ok := pathParamsMap[handler]; ok {
		for key, value := range params {
			path = strings.Replace(path, ":"+key, "%v", -1)
			values = append(values, "req."+value)
		}
	}
	if len(values) > 0 {
		return fmt.Sprintf(`fmt.Sprintf("%s", %s)`, path, strings.Join(values, ", "))
	} else {
		return fmt.Sprintf(`"%s"`, path)
	}
}

func formatGroupName(groupName string) string {
	groupNames := strings.Split(groupName, "/")
	for i, name := range groupNames {
		groupNames[i] = strings.Title(name)
	}
	return strings.Join(groupNames, "")
}
