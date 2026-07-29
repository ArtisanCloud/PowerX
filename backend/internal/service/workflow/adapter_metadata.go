package workflow

import (
	"context"
	"errors"
)

var ErrMetadataClassifierUnavailable = errors.New("workflow.metadata_classifier_unavailable")

type MetadataClassifyRequest struct {
	TenantUUID            string
	TaxonomyNamespace     string
	TagNamespace          string
	DictionaryNamespace   string
	ResourceTypeNamespace string
	Config                map[string]any
	Input                 map[string]any
}

type MetadataClassifyResponse struct {
	Output map[string]any
}

type MetadataClassifier interface {
	ClassifyMetadata(ctx context.Context, req MetadataClassifyRequest) (MetadataClassifyResponse, error)
}

type MetadataAdapter struct {
	classifier MetadataClassifier
}

func NewMetadataAdapter(classifier MetadataClassifier) *MetadataAdapter {
	return &MetadataAdapter{classifier: classifier}
}

func (a *MetadataAdapter) Spec() NodeAdapterSpec {
	return NodeAdapterSpec{
		NodeKind:    "metadata.classify",
		DisplayName: "workflow.node.metadata.classify",
		Category:    "metadata",
		InputSchema: requiredObjectSchema(
			"taxonomy_namespace",
			"tag_namespace",
			"dictionary_namespace",
			"resource_type_namespace",
			"input_path",
			"output_path",
		),
		OutputSchema: objectSchema(),
	}
}

func (a *MetadataAdapter) Validate(step StepDefinition) error {
	for _, key := range []string{"taxonomy_namespace", "tag_namespace", "dictionary_namespace", "resource_type_namespace", "input_path", "output_path"} {
		if err := requireConfigString(step, key); err != nil {
			return err
		}
	}
	return nil
}

func (a *MetadataAdapter) Execute(ctx context.Context, exec NodeExecutionContext) (NodeResult, error) {
	if a == nil || a.classifier == nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: ErrMetadataClassifierUnavailable.Error()}, ErrMetadataClassifierUnavailable
	}
	resp, err := a.classifier.ClassifyMetadata(ctx, MetadataClassifyRequest{
		TenantUUID:            exec.TenantUUID,
		TaxonomyNamespace:     configString(exec.Step.Config, "taxonomy_namespace"),
		TagNamespace:          configString(exec.Step.Config, "tag_namespace"),
		DictionaryNamespace:   configString(exec.Step.Config, "dictionary_namespace"),
		ResourceTypeNamespace: configString(exec.Step.Config, "resource_type_namespace"),
		Config:                cloneMap(exec.Step.Config),
		Input:                 cloneMap(exec.Payload),
	})
	if err != nil {
		return NodeResult{Status: NodeResultStatusFailed, ErrorCode: "workflow.metadata_classify_failed", ErrorMessage: err.Error()}, err
	}
	return NodeResult{Status: NodeResultStatusSucceeded, Output: cloneMap(resp.Output)}, nil
}
