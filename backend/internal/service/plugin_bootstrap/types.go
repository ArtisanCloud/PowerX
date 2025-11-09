package plugin_bootstrap

import (
	"encoding/json"
)

// BootstrapValidateInput represents the payload received from CLI/UI to validate a new plugin scaffold.
type BootstrapValidateInput struct {
	TemplateID string   `json:"templateId"`
	PluginID   string   `json:"pluginId"`
	CLIVersion string   `json:"cliVersion"`
	ModulePath string   `json:"modulePath"`
	GitHost    string   `json:"gitHost"`
	Owners     []string `json:"owners"`
	Visibility string   `json:"visibility"`
}

// BootstrapValidateResult conveys the validation outcome plus recommended actions.
type BootstrapValidateResult struct {
	Status          string           `json:"status"`
	PluginID        string           `json:"pluginId"`
	ModulePath      string           `json:"modulePath"`
	Template        TemplateSummary  `json:"template"`
	Issues          []BootstrapIssue `json:"issues"`
	Recommendations []string         `json:"recommendations"`
}

// BootstrapIssue captures validation warnings/errors.
type BootstrapIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // error|warning
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

// EnvironmentCheckInput represents doctor payload.
type EnvironmentCheckInput struct {
	TemplateID      string            `json:"templateId"`
	RuntimeVersions map[string]string `json:"runtimeVersions"`
	Tools           map[string]bool   `json:"tools"`
}

// EnvironmentCheckReport summarises doctor result.
type EnvironmentCheckReport struct {
	Template TemplateSummary    `json:"template"`
	Passed   bool               `json:"passed"`
	Issues   []EnvironmentIssue `json:"issues"`
}

// EnvironmentIssue represents missing binary / mismatched runtime.
type EnvironmentIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // error|warning
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
	Target   string `json:"target,omitempty"`
}

// TemplateSummary is a sanitized view returned to clients.
type TemplateSummary struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Tags          []string         `json:"tags"`
	Capabilities  []string         `json:"capabilities"`
	Backend       TemplateRuntime  `json:"backend"`
	Frontend      *TemplateRuntime `json:"frontend,omitempty"`
	CLI           TemplateCLI      `json:"cli"`
	Tooling       TemplateTooling  `json:"tooling"`
	Recommended   []string         `json:"recommended"`
	RawExtensions map[string]any   `json:"extensions,omitempty"`
}

// TemplateIndex is the YAML representation of the registry.
type TemplateIndex struct {
	Templates []TemplateSpec `yaml:"templates"`
}

// TemplateSpec describes a scaffold template and its requirements.
type TemplateSpec struct {
	ID           string           `yaml:"id" json:"id"`
	Name         string           `yaml:"name" json:"name"`
	Description  string           `yaml:"description" json:"description"`
	CLI          TemplateCLI      `yaml:"cli" json:"cli"`
	Backend      TemplateRuntime  `yaml:"backend" json:"backend"`
	Frontend     *TemplateRuntime `yaml:"frontend,omitempty" json:"frontend,omitempty"`
	Tooling      TemplateTooling  `yaml:"tooling" json:"tooling"`
	Capabilities []string         `yaml:"capabilities" json:"capabilities"`
	Tags         []string         `yaml:"tags" json:"tags"`
	Recommended  []string         `yaml:"recommended" json:"recommended"`
}

// TemplateCLI constrains CLI version requirements.
type TemplateCLI struct {
	MinVersion string `yaml:"min_version" json:"minVersion"`
}

// TemplateRuntime describes backend/frontend runtime constraints.
type TemplateRuntime struct {
	Language    string `yaml:"language" json:"language"`
	Framework   string `yaml:"framework" json:"framework"`
	TemplateRef string `yaml:"template" json:"template"`
	MinVersion  string `yaml:"min_version" json:"minVersion"`
}

// TemplateTooling lists required and optional binaries.
type TemplateTooling struct {
	Required []string `yaml:"required" json:"required"`
}

// CloneSummary copies the stable fields used for responses.
func (spec TemplateSpec) CloneSummary() TemplateSummary {
	var frontend *TemplateRuntime
	if spec.Frontend != nil {
		tmp := *spec.Frontend
		frontend = &tmp
	}
	return TemplateSummary{
		ID:           spec.ID,
		Name:         spec.Name,
		Description:  spec.Description,
		Tags:         append([]string(nil), spec.Tags...),
		Capabilities: append([]string(nil), spec.Capabilities...),
		Backend:      spec.Backend,
		Frontend:     frontend,
		CLI:          spec.CLI,
		Tooling:      spec.Tooling,
		Recommended:  append([]string(nil), spec.Recommended...),
	}
}

func marshalJSON(v any) []byte {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}
