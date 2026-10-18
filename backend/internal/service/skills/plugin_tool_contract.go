package skills

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/getkin/kin-openapi/openapi3"
)

const PluginToolContractSchema = "powerx.plugin-tool-contract/v1"

type PluginToolField struct {
	FieldKey string `json:"field_key"`
	LabelKey string `json:"label_key"`
	Unit     string `json:"unit"`
	Scope    string `json:"scope"`
}
type PluginToolSchema struct {
	ID         string                     `json:"id"`
	Definition json.RawMessage            `json:"definition"`
	Fields     map[string]PluginToolField `json:"fields"`
}

// PluginToolContractV1 当前只支持无副作用、闭合十进制字符串对象的领域计算。
// 普通插件仍返回自身 typed 输出，不实现聊天 V5，也不能声明授权/执行回执。
type PluginToolContractV1 struct {
	Schema    string                       `json:"schema"`
	ToolKey   string                       `json:"tool_key"`
	Version   string                       `json:"version"`
	Semantics string                       `json:"semantics"`
	Input     PluginToolSchema             `json:"input"`
	Output    PluginToolSchema             `json:"output"`
	Labels    map[string]map[string]string `json:"labels"`
}

type compiledPluginTool struct {
	document      PluginToolContractV1
	input, output *openapi3.Schema
}

func pluginToolDigest(raw []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) }

func readPluginToolContract(ctx context.Context, root, relative string, tool ToolDependencyV2) (*compiledPluginTool, error) {
	bad := func() (*compiledPluginTool, error) {
		return nil, fmt.Errorf("skill.tool_dependency_version_or_contract_mismatch")
	}
	if !filepath.IsLocal(relative) || !strings.HasPrefix(filepath.ToSlash(relative), "contracts/") {
		return bad()
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return bad()
	}
	defer dir.Close()
	file, err := dir.Open(relative)
	if err != nil {
		return bad()
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return bad()
	}
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 || pluginToolDigest(raw) != tool.InputSchema.SHA256 || pluginToolDigest(raw) != tool.OutputSchema.SHA256 {
		return bad()
	}
	var doc PluginToolContractV1
	if evidence.DecodeToolJSON(raw, &doc) != nil || doc.Schema != PluginToolContractSchema || doc.ToolKey != tool.ToolKey || doc.Version != tool.Version || doc.Semantics != "pure_computation" || doc.Input.ID != tool.InputSchema.ID || doc.Output.ID != tool.OutputSchema.ID || len(doc.Labels) != 2 || doc.Labels["en-US"] == nil || doc.Labels["zh-CN"] == nil {
		return bad()
	}
	for _, labels := range doc.Labels {
		if len(labels) == 0 || len(labels) > 128 {
			return bad()
		}
		for key, value := range labels {
			if key == "" || len(key) > 512 || value == "" || len(value) > 4096 {
				return bad()
			}
		}
	}
	in, err := compilePluginSchema(ctx, doc.Input, doc.Labels, false)
	if err != nil {
		return bad()
	}
	out, err := compilePluginSchema(ctx, doc.Output, doc.Labels, true)
	if err != nil {
		return bad()
	}
	return &compiledPluginTool{document: doc, input: in, output: out}, nil
}

// 第一版显式限制为完整闭合的标量对象，不忽略不支持的 JSON Schema 关键词。
func compilePluginSchema(ctx context.Context, in PluginToolSchema, labels map[string]map[string]string, output bool) (*openapi3.Schema, error) {
	var shape struct {
		Type                 string                     `json:"type"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
		Required             []string                   `json:"required"`
		Properties           map[string]json.RawMessage `json:"properties"`
	}
	if evidence.DecodeToolJSON(in.Definition, &shape) != nil || shape.Type != "object" || (shape.AdditionalProperties == nil || *shape.AdditionalProperties) || len(shape.Properties) == 0 || len(shape.Properties) > 32 || len(shape.Properties) != len(in.Fields) || len(shape.Required) != len(in.Fields) {
		return nil, fmt.Errorf("skill.tool_contract_unsupported")
	}
	seen := map[string]bool{}
	for _, name := range shape.Required {
		if seen[name] {
			return nil, fmt.Errorf("skill.tool_contract_unsupported")
		}
		seen[name] = true
	}
	fieldsSeen := map[string]bool{}
	for name, rawProperty := range shape.Properties {
		field, ok := in.Fields["/"+name]
		if !ok || !seen[name] || !regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`).MatchString(name) || !regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`).MatchString(field.FieldKey) || fieldsSeen[field.FieldKey] || field.LabelKey == "" || len(field.LabelKey) > 512 || field.Scope == "" || len(field.Scope) > 4096 || field.Unit == "" || len(field.Unit) > 512 || field.Unit == "%" || field.Unit == "％" {
			return nil, fmt.Errorf("skill.tool_contract_unsupported")
		}
		fieldsSeen[field.FieldKey] = true
		if output {
			var property struct {
				Type      string `json:"type"`
				Pattern   string `json:"pattern"`
				MaxLength int    `json:"maxLength"`
			}
			if evidence.DecodeToolJSON(rawProperty, &property) != nil || property.Type != "string" || property.Pattern != `^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$` || property.MaxLength < 1 || property.MaxLength > 80 {
				return nil, fmt.Errorf("skill.tool_contract_unsupported")
			}
		} else {
			var property struct {
				Type string `json:"type"`
			}
			if evidence.DecodeToolJSON(rawProperty, &property) != nil || property.Type != "number" {
				return nil, fmt.Errorf("skill.tool_contract_unsupported")
			}
		}
		for locale, texts := range labels {
			if (locale != "en-US" && locale != "zh-CN") || texts[field.LabelKey] == "" || len(texts[field.LabelKey]) > 4096 {
				return nil, fmt.Errorf("skill.tool_contract_unsupported")
			}
		}
	}
	var schema openapi3.Schema
	if err := json.Unmarshal(in.Definition, &schema); err != nil {
		return nil, err
	}
	if err := schema.Validate(ctx); err != nil {
		return nil, err
	}
	return &schema, nil
}

func (c *compiledPluginTool) description(agentUUID string) evidence.ToolDescription {
	desc := evidence.ToolDescription{InputFields: map[string]evidence.SourceField{}, ResultFields: map[string]evidence.SourceField{}, Labels: c.document.Labels, AgentUUID: agentUUID}
	for pointer, f := range c.document.Input.Fields {
		desc.InputFields[pointer] = evidence.SourceField{FieldKey: f.FieldKey, LabelKey: f.LabelKey, DatumKind: "quantity", Unit: f.Unit, Scope: f.Scope, NumericString: true}
	}
	for pointer, f := range c.document.Output.Fields {
		desc.ResultFields[pointer] = evidence.SourceField{FieldKey: f.FieldKey, LabelKey: f.LabelKey, DatumKind: "quantity", Unit: f.Unit, Scope: f.Scope, NumericString: true}
	}
	return desc
}
func validatePluginJSON(schema *openapi3.Schema, raw []byte) error {
	var value any
	if evidence.DecodeToolJSON(raw, &value) != nil {
		return fmt.Errorf("evidence.tool_result_invalid")
	}
	return schema.VisitJSON(value)
}
