package global

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

var G_Server *http.Server
var G_Router *gin.Engine
