package tests

import (
	"bytes"
	service "github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/boostrap/cache"
	"github.com/ArtisanCloud/PowerX/boostrap/cache/global"
	"github.com/ArtisanCloud/PowerX/config/app"
	cacheConfig "github.com/ArtisanCloud/PowerX/config/cache"
	databaseConfig "github.com/ArtisanCloud/PowerX/config/database"
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
	if config.AppConfigure == nil {

		configName := "environment"
		app.LoadEnvConfig(&envPath, &configName, nil)
		if config.AppConfigure == nil {
			logger.Logger.Error("app configure failed")
		}
		// setup ssh key path
		service.SetupSSHKeyPath(config.AppConfigure.SSH)
	}

	// Initialize the database
	if globalDatabase.G_DBConnection == nil {
		// Initialize the database

		configName := "database"
		databaseConfig.LoadDatabaseConfig(&envPath, &configName, nil)

		_ = database.SetupDatabase()
		//_ = SetupMockDatabase()
	}

	// Initialize the cache
	if global.CacheConnection == nil {

		configName := "cache"
		cacheConfig.LoadCacheConfig(&envPath, &configName, nil)

		_ = cache.SetupCache()

	}

	// load locale
	lang.LoadLanguages()
}
