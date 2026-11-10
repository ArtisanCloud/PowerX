package product

import (
	"github.com/ArtisanCloud/PowerX/internal/productinfo"
	marketsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler 提供匿名只读的产品声明和指定证明。
type Handler struct {
	svc      *marketsvc.Service
	fallback productinfo.Information
}

// Product 返回自检状态及允许公开的构建字段。
func (h *Handler) Product(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	info := h.fallback
	if h.svc != nil {
		info = h.svc.ProductInformation()
	}
	dto.ResponseSuccess(c, info)
}

// Proof 读取单个挑战，不提供枚举入口。
func (h *Handler) Proof(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	q := c.Request.URL.Query()
	id := q.Get("challenge_id")
	parsed, err := uuid.Parse(id)
	if err != nil || parsed == uuid.Nil || parsed.String() != id || len(q) != 1 || len(q["challenge_id"]) != 1 {
		dto.ResponseError(c, 400, "challenge_id 无效", nil)
		return
	}
	if h.svc == nil {
		dto.ResponseError(c, 503, "证明服务暂不可用", nil)
		return
	}
	proof, ok := h.svc.MarketplaceProof(id)
	if !ok {
		dto.ResponseError(c, 404, "指定证明不可用", nil)
		return
	}
	dto.ResponseSuccess(c, proof)
}
