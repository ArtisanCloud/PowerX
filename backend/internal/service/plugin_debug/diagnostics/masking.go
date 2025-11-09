package diagnostics

import (
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type maskRule struct {
	Name        string `yaml:"name"`
	Pattern     string `yaml:"pattern"`
	Replacement string `yaml:"replacement"`
	regex       *regexp.Regexp
}

// Masker applies regex-based replacements to arbitrary structures.
type Masker struct {
	rules []maskRule
}

type maskingConfig struct {
	Rules []maskRule `yaml:"rules"`
}

// LoadMasker constructs a masker from yaml file.
func LoadMasker(path string) (*Masker, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg maskingConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	for i := range cfg.Rules {
		if cfg.Rules[i].Pattern == "" {
			continue
		}
		re, err := regexp.Compile(cfg.Rules[i].Pattern)
		if err != nil {
			return nil, err
		}
		cfg.Rules[i].regex = re
		if cfg.Rules[i].Replacement == "" {
			cfg.Rules[i].Replacement = "***"
		}
	}
	return &Masker{rules: cfg.Rules}, nil
}

func (m *Masker) maskString(val string) (string, bool) {
	if m == nil {
		return val, false
	}
	masked := val
	changed := false
	for _, rule := range m.rules {
		if rule.regex == nil {
			continue
		}
		replaced := rule.regex.ReplaceAllString(masked, rule.Replacement)
		if replaced != masked {
			masked = replaced
			changed = true
		}
	}
	return masked, changed
}

func (m *Masker) MaskMap(data map[string]any) (map[string]any, bool) {
	if m == nil || data == nil {
		return data, false
	}
	changed := false
	out := make(map[string]any, len(data))
	for k, v := range data {
		switch val := v.(type) {
		case string:
			if masked, ok := m.maskString(val); ok {
				out[k] = masked
				changed = true
			} else {
				out[k] = val
			}
		case map[string]any:
			if maskedMap, ok := m.MaskMap(val); ok {
				out[k] = maskedMap
				changed = true
			} else {
				out[k] = maskedMap
			}
		case []any:
			if maskedSlice, ok := m.MaskSlice(val); ok {
				out[k] = maskedSlice
				changed = true
			} else {
				out[k] = maskedSlice
			}
		default:
			out[k] = v
		}
	}
	return out, changed
}

func (m *Masker) MaskSlice(items []any) ([]any, bool) {
	if m == nil || items == nil {
		return items, false
	}
	changed := false
	out := make([]any, len(items))
	for i, item := range items {
		switch val := item.(type) {
		case string:
			if masked, ok := m.maskString(val); ok {
				out[i] = masked
				changed = true
			} else {
				out[i] = val
			}
		case map[string]any:
			if maskedMap, ok := m.MaskMap(val); ok {
				out[i] = maskedMap
				changed = true
			} else {
				out[i] = maskedMap
			}
		default:
			out[i] = item
		}
	}
	return out, changed
}

func (m *Masker) MaskStringMap(data map[string]string) (map[string]string, bool) {
	if m == nil || data == nil {
		return data, false
	}
	changed := false
	out := make(map[string]string, len(data))
	for k, v := range data {
		if masked, ok := m.maskString(v); ok {
			out[k] = masked
			changed = true
		} else {
			out[k] = v
		}
	}
	return out, changed
}

func (m *Masker) MaskStrings(values []string) ([]string, bool) {
	if m == nil || values == nil {
		return values, false
	}
	out := make([]string, len(values))
	changed := false
	for i, v := range values {
		if masked, ok := m.maskString(v); ok {
			out[i] = masked
			changed = true
		} else {
			out[i] = v
		}
	}
	return out, changed
}
