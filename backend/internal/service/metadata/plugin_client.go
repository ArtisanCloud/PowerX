package metadata

import (
	"context"
	"errors"
	"strings"
)

const (
	CapabilityMetadataDictionaryRead   = "com.corex.metadata.dictionary.read"
	CapabilityMetadataTaxonomyRead     = "com.corex.metadata.taxonomy.read"
	CapabilityMetadataTagRead          = "com.corex.metadata.tag.read"
	CapabilityMetadataTagManage        = "com.corex.metadata.tag.manage"
	CapabilityMetadataResourceTypeRead = "com.corex.metadata.resource_type.read"
)

type MetadataCapabilityInvoker interface {
	InvokeMetadataCapability(ctx context.Context, capabilityID string, payload map[string]any) (map[string]any, error)
}

type PluginMetadataClient struct {
	mode    string
	invoker MetadataCapabilityInvoker
	seed    *SeedFile
}

type PluginMetadataClientOptions struct {
	Mode     string
	Invoker  MetadataCapabilityInvoker
	SeedPath string
}

func NewPluginMetadataClient(opts PluginMetadataClientOptions) (*PluginMetadataClient, error) {
	mode := strings.TrimSpace(opts.Mode)
	if mode == "" {
		mode = "delegated"
	}
	switch mode {
	case "delegated":
		if opts.Invoker == nil {
			return nil, errors.New("metadata delegated client requires capability invoker")
		}
		return &PluginMetadataClient{mode: mode, invoker: opts.Invoker}, nil
	case "local":
		seedPath := strings.TrimSpace(opts.SeedPath)
		if seedPath == "" {
			seedPath = DefaultSeedPath
		}
		seed, err := LoadSeedFile(seedPath)
		if err != nil {
			return nil, err
		}
		if err := ValidateCanonicalSeedDefinitions(seed); err != nil {
			return nil, err
		}
		return &PluginMetadataClient{mode: mode, seed: &seed}, nil
	default:
		return nil, errors.New("metadata client mode must be delegated or local")
	}
}

func (c *PluginMetadataClient) ResolveResourceType(ctx context.Context, resourceType string) (map[string]any, error) {
	return c.invoke(ctx, CapabilityMetadataResourceTypeRead, restPayload("GET", "/api/v1/admin/metadata/resource-types", map[string]any{
		"q": strings.TrimSpace(resourceType),
	}, nil))
}

func (c *PluginMetadataClient) ListDictionaryItems(ctx context.Context, namespaceUUID string, query map[string]any) (map[string]any, error) {
	return c.invoke(ctx, CapabilityMetadataDictionaryRead, restPayload("GET", "/api/v1/admin/metadata/dictionaries/"+strings.TrimSpace(namespaceUUID)+"/items", query, nil))
}

func (c *PluginMetadataClient) ListTaxonomyNodes(ctx context.Context, taxonomyUUID string, query map[string]any) (map[string]any, error) {
	return c.invoke(ctx, CapabilityMetadataTaxonomyRead, restPayload("GET", "/api/v1/admin/metadata/taxonomies/"+strings.TrimSpace(taxonomyUUID)+"/nodes", query, nil))
}

func (c *PluginMetadataClient) ListTags(ctx context.Context, query map[string]any) (map[string]any, error) {
	return c.invoke(ctx, CapabilityMetadataTagRead, restPayload("GET", "/api/v1/admin/metadata/tags", query, nil))
}

func (c *PluginMetadataClient) ReplaceTagBindings(ctx context.Context, resourceType string, resourceUUID string, tagUUIDs []string) (map[string]any, error) {
	return c.invoke(ctx, CapabilityMetadataTagManage, restPayload("PUT", "/api/v1/admin/metadata/tag-bindings", nil, map[string]any{
		"resource_type": strings.TrimSpace(resourceType),
		"resource_uuid": strings.TrimSpace(resourceUUID),
		"tag_uuids":     tagUUIDs,
	}))
}

func (c *PluginMetadataClient) LocalSeed() (SeedFile, error) {
	if c == nil || c.mode != "local" || c.seed == nil {
		return SeedFile{}, errors.New("metadata local seed is unavailable")
	}
	return *c.seed, nil
}

func (c *PluginMetadataClient) invoke(ctx context.Context, capabilityID string, payload map[string]any) (map[string]any, error) {
	if c == nil || c.mode != "delegated" || c.invoker == nil {
		return nil, errors.New("metadata delegated capability invoker is unavailable")
	}
	return c.invoker.InvokeMetadataCapability(ctx, capabilityID, payload)
}

func restPayload(method string, endpoint string, query map[string]any, body map[string]any) map[string]any {
	payload := map[string]any{
		"method":   method,
		"endpoint": endpoint,
	}
	if len(query) > 0 {
		payload["query"] = query
	}
	if len(body) > 0 {
		payload["body"] = body
	}
	return payload
}
