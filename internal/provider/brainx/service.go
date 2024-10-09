package brainx

import (
	"PowerX/internal/logic/openapi/provider/brainx/schema"
	"PowerX/internal/svc"
	"context"
	"encoding/json"
)

type BrainXServiceProvider struct {
	Client *BrainXProviderClient
}

func NewBrainXServiceProvider(svcCtx *svc.ServiceContext) *BrainXServiceProvider {

	return &BrainXServiceProvider{
		Client: NewBrainXProviderClient(&svcCtx.Config, svcCtx.PowerX.Cache),
	}
}

func (sp *BrainXServiceProvider) HelloWorld(ctx context.Context) (message string, err error) {
	url := "/demo/hello-world"
	jsonBody := map[string]string{
		"name":    "powerx",
		"message": "hello",
	}
	resp, err := sp.Client.HTTPPost(ctx, url, jsonBody, true, nil)
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

func (sp *BrainXServiceProvider) EchoLongTime(ctx context.Context, timeout int) (message string, err error) {
	url := "/demo/echo-long-time"
	jsonBody := map[string]int{
		"timeout": timeout,
	}

	resp, err := sp.Client.HTTPPost(ctx, url, jsonBody, true, nil)

	if err != nil {
		return "", err
	}

	result := schema.ResponseEchoLongTime{}
	err = json.Unmarshal(resp, &result)
	if err != nil {
		return "", err
	}

	return result.Message, err
}
