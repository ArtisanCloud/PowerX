package providerclient

type ProviderClientInterface interface {
	Auth() error
	GetAccessToken() (string, error)
	HTTPGet(url string, params map[string]string, useAuth bool, headers map[string]string) (map[string]interface{}, error)
	HTTPPost(url string, jsonData interface{}, useAuth bool, headers map[string]string) (map[string]interface{}, error)
}
