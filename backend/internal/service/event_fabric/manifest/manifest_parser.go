package manifest

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
)

// Manifest describes the declarative topic + ACL requirements for a plugin.
type Manifest struct {
	Version  int          `yaml:"version" json:"version"`
	Defaults TopicDefault `yaml:"defaults" json:"defaults"`
	Topics   []TopicSpec  `yaml:"topics" json:"topics"`
}

// TopicDefault captures fallback values when a topic omits specific fields.
type TopicDefault struct {
	PayloadFormat   string                 `yaml:"payload_format" json:"payload_format"`
	VersioningMode  string                 `yaml:"versioning_mode" json:"versioning_mode"`
	MaxRetry        int                    `yaml:"max_retry" json:"max_retry"`
	AckTimeoutSec   int                    `yaml:"ack_timeout_seconds" json:"ack_timeout_seconds"`
	RetentionPolicy map[string]interface{} `yaml:"retention_policy" json:"retention_policy"`
	Metadata        map[string]interface{} `yaml:"metadata" json:"metadata"`
}

// TopicSpec defines a single topic entry in the manifest.
type TopicSpec struct {
	Key             string                 `yaml:"key" json:"key"`
	Namespace       string                 `yaml:"namespace" json:"namespace"`
	Name            string                 `yaml:"name" json:"name"`
	PayloadFormat   string                 `yaml:"payload_format" json:"payload_format"`
	VersioningMode  string                 `yaml:"versioning_mode" json:"versioning_mode"`
	MaxRetry        *int                   `yaml:"max_retry" json:"max_retry"`
	AckTimeoutSec   *int                   `yaml:"ack_timeout_seconds" json:"ack_timeout_seconds"`
	RetentionPolicy map[string]interface{} `yaml:"retention_policy" json:"retention_policy"`
	Metadata        map[string]interface{} `yaml:"metadata" json:"metadata"`
	Principals      []PrincipalSpec        `yaml:"acl" json:"acl"`
}

// PrincipalSpec describes the ACL binding that should be created for a topic.
type PrincipalSpec struct {
	PrincipalType string   `yaml:"principal_type" json:"principal_type"`
	PrincipalID   string   `yaml:"principal_id" json:"principal_id"`
	Actions       []string `yaml:"actions" json:"actions"`
}

// SeedContext provides runtime data when rendering a manifest for a specific tenant.
type SeedContext struct {
	TenantUUID    string
	PluginID      string
	PluginVersion string
	Operator      string
	Variables     map[string]string
}

// SeedPlan is the evaluation result of a manifest.
type SeedPlan struct {
	Topics []TopicPlan
}

// TopicPlan describes a topic + ACL items to be applied.
type TopicPlan struct {
	Key       string
	FullTopic string
	Topic     directory.CreateTopicInput
	ACL       []ACLPlan
}

// ACLPlan contains resolved ACL grant data for a topic.
type ACLPlan struct {
	TopicFullName string
	PrincipalType string
	PrincipalID   string
	Actions       []acl.PrincipalAction
}

// Parse loads a manifest from a reader.
func Parse(r io.Reader) (*Manifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Load(data)
}

// Load loads a manifest from bytes.
func Load(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest failed: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// Validate ensures the manifest contains at least one topic with minimum data.
func (m *Manifest) Validate() error {
	if m.Version <= 0 {
		return fmt.Errorf("manifest version must be positive")
	}
	if len(m.Topics) == 0 {
		return fmt.Errorf("manifest requires at least one topic")
	}
	keys := make(map[string]struct{})
	for idx, topic := range m.Topics {
		if strings.TrimSpace(topic.Namespace) == "" {
			return fmt.Errorf("topic[%d] namespace is required", idx)
		}
		if strings.TrimSpace(topic.Name) == "" {
			return fmt.Errorf("topic[%d] name is required", idx)
		}
		key := topic.Key
		if key == "" {
			key = fmt.Sprintf("%s.%s", topic.Namespace, topic.Name)
		}
		if _, exists := keys[key]; exists {
			return fmt.Errorf("duplicate topic key detected: %s", key)
		}
		keys[key] = struct{}{}
	}
	return nil
}

// Render converts the manifest into concrete topic + ACL plans using runtime context.
func (m *Manifest) Render(ctx SeedContext) (*SeedPlan, error) {
	tenant := strings.TrimSpace(ctx.TenantUUID)
	if tenant == "" {
		return nil, fmt.Errorf("tenant uuid is required")
	}

	data := map[string]string{
		"tenant_uuid":    tenant,
		"plugin_id":      ctx.PluginID,
		"plugin_version": ctx.PluginVersion,
	}
	if ctx.PluginID != "" {
		data["plugin.id"] = ctx.PluginID
	}
	if ctx.PluginVersion != "" {
		data["plugin.version"] = ctx.PluginVersion
	}
	data["tenant.uuid"] = tenant
	for k, v := range ctx.Variables {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		key := strings.ToLower(k)
		data[key] = v
		data[fmt.Sprintf("variables.%s", key)] = v
	}

	defaults := m.Defaults
	plan := &SeedPlan{Topics: make([]TopicPlan, 0, len(m.Topics))}
	for idx, spec := range m.Topics {
		namespace, err := renderToken(spec.Namespace, data)
		if err != nil {
			return nil, fmt.Errorf("topic[%d] namespace: %w", idx, err)
		}
		name, err := renderToken(spec.Name, data)
		if err != nil {
			return nil, fmt.Errorf("topic[%d] name: %w", idx, err)
		}
		namespace = normalizeSegment(namespace)
		name = normalizeSegment(name)
		if namespace == "" || name == "" {
			return nil, fmt.Errorf("topic[%d] namespace/name cannot be empty", idx)
		}

		payloadFormat := coalesceString(spec.PayloadFormat, defaults.PayloadFormat, "json")
		versioningMode := coalesceString(spec.VersioningMode, defaults.VersioningMode, "strict")
		maxRetry := coalesceInt(spec.MaxRetry, defaults.MaxRetry, 5)
		ackTimeout := coalesceInt(spec.AckTimeoutSec, defaults.AckTimeoutSec, 30)

		retention := chooseMap(spec.RetentionPolicy, defaults.RetentionPolicy)
		retentionJSON, err := encodeMap(retention)
		if err != nil {
			return nil, fmt.Errorf("topic[%d] retention_policy: %w", idx, err)
		}

		metadata := chooseMap(spec.Metadata, defaults.Metadata)

		create := directory.CreateTopicInput{
			TenantUUID:      tenant,
			Namespace:       namespace,
			Name:            name,
			PayloadFormat:   payloadFormat,
			VersioningMode:  versioningMode,
			MaxRetry:        int32(maxRetry),
			AckTimeoutSec:   int32(ackTimeout),
			RetentionPolicy: retentionJSON,
			Metadata:        metadata,
			CreatedBy:       ctx.Operator,
		}

		topicKey := spec.Key
		if strings.TrimSpace(topicKey) == "" {
			topicKey = fmt.Sprintf("%s.%s", namespace, name)
		}
		fullTopic := fmt.Sprintf("%s.%s.%s", tenant, namespace, name)

		aclPlans := make([]ACLPlan, 0, len(spec.Principals))
		for pIdx, principal := range spec.Principals {
			renderedID, err := renderToken(principal.PrincipalID, data)
			if err != nil {
				return nil, fmt.Errorf("topic[%d].acl[%d] principal_id: %w", idx, pIdx, err)
			}
			renderedID = strings.TrimSpace(renderedID)
			if renderedID == "" {
				return nil, fmt.Errorf("topic[%d].acl[%d] principal_id cannot be empty", idx, pIdx)
			}
			principalType := strings.ToLower(strings.TrimSpace(principal.PrincipalType))
			if principalType == "" {
				return nil, fmt.Errorf("topic[%d].acl[%d] principal_type is required", idx, pIdx)
			}
			actions, err := normalizeActions(principal.Actions)
			if err != nil {
				return nil, fmt.Errorf("topic[%d].acl[%d] actions: %w", idx, pIdx, err)
			}
			aclPlans = append(aclPlans, ACLPlan{
				TopicFullName: fullTopic,
				PrincipalType: principalType,
				PrincipalID:   renderedID,
				Actions:       actions,
			})
		}

		plan.Topics = append(plan.Topics, TopicPlan{
			Key:       topicKey,
			FullTopic: fullTopic,
			Topic:     create,
			ACL:       aclPlans,
		})
	}

	return plan, nil
}

func (m *Manifest) TopicCount() int {
	return len(m.Topics)
}

func renderToken(raw string, data map[string]string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.Contains(raw, "{{") {
		return raw, nil
	}
	var out strings.Builder
	for {
		start := strings.Index(raw, "{{")
		if start == -1 {
			out.WriteString(raw)
			break
		}
		out.WriteString(raw[:start])
		raw = raw[start+2:]
		end := strings.Index(raw, "}}")
		if end == -1 {
			return "", fmt.Errorf("unclosed template token")
		}
		token := strings.TrimSpace(raw[:end])
		raw = raw[end+2:]
		value, ok := data[strings.ToLower(token)]
		if !ok {
			value, ok = data[token]
		}
		if !ok {
			return "", fmt.Errorf("variable %s not defined", token)
		}
		out.WriteString(value)
	}
	return out.String(), nil
}

func coalesceString(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func coalesceInt(value *int, fallback int, defaultVal int) int {
	if value != nil && *value > 0 {
		return *value
	}
	if fallback > 0 {
		return fallback
	}
	return defaultVal
}

func chooseMap(primary, fallback map[string]interface{}) map[string]interface{} {
	if primary != nil {
		return cloneMap(primary)
	}
	if fallback != nil {
		return cloneMap(fallback)
	}
	return nil
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func encodeMap(value map[string]interface{}) (string, error) {
	if value == nil {
		return "{}", nil
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func normalizeSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "")
	return strings.ToLower(value)
}

func normalizeActions(actions []string) ([]acl.PrincipalAction, error) {
	if len(actions) == 0 {
		return nil, fmt.Errorf("actions cannot be empty")
	}
	normalized := make([]acl.PrincipalAction, 0, len(actions))
	for _, action := range actions {
		token := strings.ToLower(strings.TrimSpace(action))
		if token == "" {
			return nil, fmt.Errorf("action cannot be empty")
		}
		switch token {
		case string(acl.PrincipalActionPublish), string(acl.PrincipalActionSubscribe), string(acl.PrincipalActionReplay):
			normalized = append(normalized, acl.PrincipalAction(token))
		default:
			return nil, fmt.Errorf("unknown action %s", token)
		}
	}
	return normalized, nil
}
