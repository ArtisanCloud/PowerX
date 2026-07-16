package metadata

import "context"

type ResourceValidator interface {
	ValidateResource(ctx context.Context, tenantUUID string, resourceUUID string) error
}

type ResourceValidatorRegistry interface {
	Get(resourceType string) (ResourceValidator, bool)
}

type StaticResourceValidatorRegistry struct {
	validators map[string]ResourceValidator
}

func NewStaticResourceValidatorRegistry(validators map[string]ResourceValidator) *StaticResourceValidatorRegistry {
	if validators == nil {
		validators = map[string]ResourceValidator{}
	}
	return &StaticResourceValidatorRegistry{validators: validators}
}

func (r *StaticResourceValidatorRegistry) Get(resourceType string) (ResourceValidator, bool) {
	if r == nil {
		return nil, false
	}
	v, ok := r.validators[resourceType]
	return v, ok
}
