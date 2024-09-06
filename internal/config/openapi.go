package config

import (
	"PowerX/internal/config/openapiplatform"
	"PowerX/internal/config/openapiprovider"
)

type OpenAPI struct {
	Platforms struct {
		BrainX openapiplatform.BrainX
	}
	Providers struct {
		BrainX openapiprovider.BrainX
	}
}
