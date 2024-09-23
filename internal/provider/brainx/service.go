package brainx

import (
	"PowerX/internal/logic/openapi/provider/brainx/schema"
	"PowerX/internal/svc"
	"encoding/json"
)

type BrainXServiceProvider struct {
	client *BrainXProviderClient
}

func NewBrainXServiceProvider(svcCtx *svc.ServiceContext) *BrainXServiceProvider {

	return &BrainXServiceProvider{
		client: NewBrainXProviderClient(&svcCtx.Config, svcCtx.PowerX.Cache),
	}
}

func (sp *BrainXServiceProvider) HelloWorld() (message string, err error) {
	url := "/demo/hello-world"
	jsonBody := map[string]string{
		"name":    "powerx",
		"message": "hello",
	}
	resp, err := sp.client.HTTPPost(url, jsonBody, true, nil)
	if err != nil {
		return "", err
	}

	result := schema.ResponseHelloWorld{}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return "", err
	}

	return result.Message, err
}
