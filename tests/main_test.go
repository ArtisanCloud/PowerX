package tests

import (
	"bytes"
	service "github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/boostrap/cache"
	"github.com/ArtisanCloud/PowerX/boostrap/cache/global"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/database"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/ArtisanCloud/PowerX/resources/lang"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestMain(m *testing.M) {

	logger.Logger.Info("Before Test: [++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++")

	// init test app
	SetupTestEnv(m)

	exitVal := m.Run()

	logger.Logger.Info("After Test: ------------------------------------------------------------------]")

	os.Exit(exitVal)

}

func TestInit(t *testing.T) {
}

func MockGin(action string, url string, requestData url.Values) (*httptest.ResponseRecorder, *gin.Context) {

	if action == "" {
		action = "POST"
	}
	if url == "" {
		url = "/"
	}

	var (
		body io.Reader
		//request http.Request
	)

	if requestData != nil {
		//body = strings.NewReader(requestData.Encode())
		body = bytes.NewBufferString(requestData.Encode())
	} else {
		body = nil
	}

	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	context.Request, _ = http.NewRequest(action, url, body)

	return writer, context
}

func SetupTestEnv(t *testing.M) {

	envPath := "../"

	// Initialize the global config
	if config.G_AppConfigure == nil {

		err := config.LoadEnvConfig(&envPath)
		if err != nil {
			logger.Logger.Error(err.Error())
		}
		if config.G_AppConfigure == nil {
			logger.Logger.Error("app configure failed")
		}
		// setup ssh key path
		service.SetupSSHKeyPath(&config.G_AppConfigure.SSHConfig)
	}

	// Initialize the database
	if globalDatabase.G_DBConnection == nil {
		// Initialize the database

		config.LoadDatabaseConfig()

		_ = database.SetupDatabase()
		//_ = SetupMockDatabase()
	}

	// Initialize the cache
	if global.G_CacheConnection == nil {

		config.LoadCacheConfig()

		_ = cache.SetupCache()

	}

	// load locale
	lang.LoadLanguages()
}
