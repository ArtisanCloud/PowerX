package seed

import "os"

func resolveSwaggerPath() string {
	candidates := []string{
		"./api/openapi/swagger.json",
		"./backend/api/openapi/swagger.json",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return candidates[0]
}
