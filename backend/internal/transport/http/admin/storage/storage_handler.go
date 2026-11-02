package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func fail(c *gin.Context, status int, code, message string) {
	dto.RespondErrorFrom(c, dto.NewErrorWithCode(status, code, message, nil))
}

func requireRootUser(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	claims := reqctx.GetClaims(c.Request.Context())
	allowed := claims != nil && reqctx.IsRoot(c.Request.Context()) && claims.UserID != 0 && claims.Issuer != "powerx-sts"
	if claims != nil {
		for _, plane := range claims.Platforms {
			if plane == "api_key" || plane == "service_actor" {
				allowed = false
			}
		}
	}
	if !allowed {
		fail(c, http.StatusForbidden, "STORAGE_ROOT_REQUIRED", "存储治理仅允许平台 Root 用户会话")
		c.Abort()
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		fail(c, 400, "STORAGE_QUERY_FORBIDDEN", "存储观测不接受路径或范围覆盖参数")
		c.Abort()
		return
	}
	c.Next()
}

type handler struct{ service *storagegov.Service }

func (h *handler) overview(c *gin.Context) {
	if h.service == nil {
		fail(c, 503, "STORAGE_OBSERVER_UNAVAILABLE", "存储观测未就绪，请检查部署配置与 Core 版本")
		return
	}
	out, err := h.service.Overview(c.Request.Context())
	if err != nil {
		fail(c, 503, "STORAGE_OBSERVATION_FAILED", "读取存储观测失败，请重试")
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) estimate(c *gin.Context) {
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	if err != nil {
		fail(c, 400, "STORAGE_ESTIMATE_INVALID", "容量测算请求无效")
		return
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		fail(c, 400, "STORAGE_ESTIMATE_INVALID", "容量测算必须是 JSON 对象")
		return
	}
	var in storagegov.EstimateInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		fail(c, 400, "STORAGE_ESTIMATE_INVALID", "容量测算字段或格式无效")
		return
	}
	var out storagegov.Estimate
	if in.ResourceID != "" {
		if h.service == nil {
			fail(c, 503, "STORAGE_OBSERVER_UNAVAILABLE", "来源目录未就绪")
			return
		}
		out, err = h.service.EstimateResource(in)
	} else {
		out, err = storagegov.CalculateEstimate(in)
	}
	if err != nil {
		fail(c, 400, "STORAGE_ESTIMATE_INVALID", "天数、容量和增量必须在支持范围内")
		return
	}
	dto.ResponseSuccess(c, out)
}
