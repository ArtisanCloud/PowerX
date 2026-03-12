package ai

import (
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// Register mounts AI public APIs under /ai/*.
func Register(public, protected *gin.RouterGroup, deps *shared.Deps) {
	if protected == nil || deps == nil {
		return
	}
	handler := newAIHandler(deps)
	group := protected.Group("/ai")

	group.POST("/llm/invoke", handler.llmInvoke)
	group.GET("/llm/models", handler.llmModelsList)
	group.POST("/llm/sessions", handler.llmSessionCreate)
	group.POST("/llm/sessions/:session_id/messages", handler.llmSessionAppend)
	group.GET("/llm/sessions/:session_id/stream", handler.llmSessionStream)

	group.POST("/vlm/invoke", handler.vlmInvoke)
	group.POST("/image/invoke", handler.imageInvoke)
	group.POST("/video/invoke", handler.videoInvoke)
	group.POST("/tts/invoke", handler.ttsInvoke)
	group.POST("/embedding/invoke", handler.embeddingInvoke)

	if public != nil {
		public.GET("/ai/status", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	}
}
