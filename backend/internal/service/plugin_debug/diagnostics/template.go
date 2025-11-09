package diagnostics

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ReportTemplate defines sections used in rendered reports.
type ReportTemplate struct {
	Version  string            `yaml:"version"`
	Metadata map[string]string `yaml:"metadata"`
	Sections []TemplateSection `yaml:"sections"`
}

// TemplateSection defines field projections for a section.
type TemplateSection struct {
	ID     string   `yaml:"id"`
	Title  string   `yaml:"title"`
	Fields []string `yaml:"fields"`
}

// LoadTemplate loads template definition from yaml if path provided.
func LoadTemplate(path string) (*ReportTemplate, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tpl ReportTemplate
	if err := yaml.Unmarshal(data, &tpl); err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (t *ReportTemplate) Render(summary map[string]any) map[string]any {
	if t == nil || len(t.Sections) == 0 {
		return summary
	}
	sections := make([]map[string]any, 0, len(t.Sections))
	for _, section := range t.Sections {
		items := make(map[string]any)
		for _, field := range section.Fields {
			if val, ok := summary[field]; ok {
				items[field] = val
			}
		}
		sections = append(sections, map[string]any{
			"id":    section.ID,
			"title": section.Title,
			"items": items,
		})
	}
	rendered := map[string]any{
		"sections": sections,
		"raw":      summary,
	}
	if len(t.Metadata) > 0 {
		rendered["meta"] = t.Metadata
	}
	return rendered
}
