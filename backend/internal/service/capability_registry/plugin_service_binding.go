package capability_registry

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"gopkg.in/yaml.v3"
)

// 新服务身份仅用于正式声明的独立服务入口，不能把 Admin 绑定自动转为服务 API。
func validateServiceTicketProtocols(protocols []models.ProtocolBinding) error {
	for _, p := range protocols {
		if p.AuthType != PluginServiceAuthType {
			continue
		}
		if p.Channel != "rest" || p.ActorContext != "service_actor" || p.ResourceScope != "tenant" || strings.TrimSpace(p.SchemaRef) == "" {
			return fmt.Errorf("invalid core_service_ticket protocol contract")
		}
		switch p.Method {
		case http.MethodPost, http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			return fmt.Errorf("invalid core_service_ticket HTTP method")
		}
		if _, err := FixedPluginServicePath(p.Endpoint); err != nil {
			return fmt.Errorf("invalid core_service_ticket path: %w", err)
		}
	}
	return nil
}

// 服务票据的本地 schema_ref 可带 JSON Pointer；文件名和片段分别校验。
func serviceTicketLocalSchemaRef(value string) (string, string, error) {
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") || u.RawQuery != "" || u.ForceQuery || u.RawPath != "" || strings.Contains(u.Path, "\\") || path.Clean(u.Path) != u.Path || u.Path == ".." || strings.HasPrefix(u.Path, "../") {
		return "", "", fmt.Errorf("invalid packaged service schema reference")
	}
	if u.Fragment != "" && !strings.HasPrefix(u.Fragment, "/") {
		return "", "", fmt.Errorf("service schema fragment must be a JSON Pointer")
	}
	return u.Path, u.Fragment, nil
}

func validateServiceTicketSchemaDocument(raw []byte, fragment string) error {
	var document any
	if err := yaml.Unmarshal(raw, &document); err != nil || document == nil {
		return fmt.Errorf("service schema document is invalid")
	}
	target := document
	if fragment != "" {
		for _, token := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
			var decoded strings.Builder
			for i := 0; i < len(token); i++ {
				if token[i] != '~' {
					decoded.WriteByte(token[i])
					continue
				}
				if i+1 >= len(token) || (token[i+1] != '0' && token[i+1] != '1') {
					return fmt.Errorf("invalid service schema JSON Pointer")
				}
				i++
				if token[i] == '0' {
					decoded.WriteByte('~')
				} else {
					decoded.WriteByte('/')
				}
			}
			key := decoded.String()
			switch value := target.(type) {
			case map[string]any:
				var exists bool
				target, exists = value[key]
				if !exists {
					return fmt.Errorf("service schema fragment does not exist")
				}
			case []any:
				index, err := strconv.Atoi(key)
				if err != nil || strconv.Itoa(index) != key || index < 0 || index >= len(value) {
					return fmt.Errorf("service schema fragment does not exist")
				}
				target = value[index]
			default:
				return fmt.Errorf("service schema fragment does not exist")
			}
		}
	}
	switch target.(type) {
	case map[string]any, bool:
		return nil
	default:
		return fmt.Errorf("service schema reference must resolve to a schema object")
	}
}

// 仅豁免当前精确匹配的票据绑定，不因能力中其他绑定使用票据而放宽路径。
func permissionBindingUsesServiceTicket(binding permissionProtocolBinding, protocols []models.ProtocolBinding) bool {
	for _, protocol := range protocols {
		if protocol.AuthType == PluginServiceAuthType && protocol.Channel == binding.Channel && protocol.Method == binding.Method && protocol.Endpoint == binding.Path {
			return true
		}
	}
	return false
}
