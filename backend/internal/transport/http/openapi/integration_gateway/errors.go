package integration_gateway

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

//go:embed locales/errors.json
var errorLocaleJSON []byte

var errorLocales = func() map[string]map[string]string {
	var values map[string]map[string]string
	if err := json.Unmarshal(errorLocaleJSON, &values); err != nil {
		panic(err)
	}
	return values
}()

func gatewayError(c *gin.Context, status int, code string) *dto.AppError {
	language := "zh-CN"
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Accept-Language")), "en") {
		language = "en-US"
	}
	return dto.NewErrorWithCode(status, code, errorLocales[language][strconv.Itoa(status)], nil)
}
