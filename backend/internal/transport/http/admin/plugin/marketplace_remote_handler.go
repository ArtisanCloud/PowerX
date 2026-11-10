package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
)

func marketPage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}
func respondMarketError(c *gin.Context, err error) {
	var upstream *market.Error
	if errors.As(err, &upstream) {
		dto.ResponseErrorWithDetails(c, upstream.Status, upstream.Message, nil, gin.H{"code": upstream.Code})
		return
	}
	if plugin_mgr.IsCode(err, plugin_mgr.CodeIOError) {
		// URL 下载错误可能包含对象存储预签名参数，不将底层错误回传浏览器。
		dto.ResponseError(c, 502, "下载安装包或解压失败，请检查市场制品与存储连接", nil)
		return
	}
	respondPluginInstallError(c, "Marketplace 安装失败", err)
}
func MarketplaceCatalogHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, size := marketPage(c)
		result, err := deps.PluginMarketplace.Catalog(c.Request.Context(), page, size, c.Query("installation"))
		if err != nil {
			respondMarketError(c, err)
			return
		}
		dto.ResponseSuccess(c, result)
	}
}
func MarketplaceVersionsHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, size := marketPage(c)
		result, err := deps.PluginMarketplace.Versions(c.Request.Context(), c.Param("plugin_id"), page, size)
		if err != nil {
			respondMarketError(c, err)
			return
		}
		dto.ResponseSuccess(c, result)
	}
}

// MarketplaceInstallHandler 只接受目录 ID 和明确版本，不接受客户端下载 URL/校验和。
// 下载后复用本地安装器及同一个租户启用收尾流程。
func MarketplaceInstallHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			PluginID              string `json:"plugin_id" validate:"required"`
			Version               string `json:"version" validate:"required"`
			Enable                bool   `json:"enable"`
			Replace               bool   `json:"replace"`
			ExpectedRevision      string `json:"expected_revision"`
			ConfirmLegacyProvider bool   `json:"confirm_legacy_provider"`
		}
		if err := dto.ValidateRequestWithContext(c, &req); err != nil {
			dto.ResponseValidationError(c, err)
			return
		}
		defer func() {
			if deps.AuditSvc == nil {
				return
			}
			outcome := "SUCCESS"
			if c.Writer.Status() >= 400 {
				outcome = "FAIL"
			}
			actor := int64(reqctx.GetUserID(c.Request.Context()))
			meta, _ := json.Marshal(map[string]any{"version": req.Version, "replace": req.Replace, "provider_binding_confirmed": req.ConfirmLegacyProvider})
			operation := "INSTALL"
			if req.Replace {
				operation = "REPLACE"
			}
			_ = deps.AuditSvc.Emit(c.Request.Context(), &auditmodel.AuditEvent{
				OccurredAt: time.Now().UTC(), TenantUUID: reqctx.GetTenantUUID(c.Request.Context()), Source: "plugin_marketplace",
				Operation: operation, ResourceType: "plugin", ResourceID: req.PluginID, Outcome: outcome, Severity: "INFO", ActorUserID: &actor, Meta: meta,
			})
		}()
		seed, err := installTenantHostConfigSeed(c)
		if err != nil {
			dto.ResponseError(c, 400, "PLUGIN_INSTALL_TENANT_CONTEXT_REQUIRED", err)
			return
		}
		mgr, err := tryGetPluginManager()
		if err != nil {
			respondPluginRuntimeUnavailable(c, err)
			return
		}
		provider, err := deps.PluginMarketplace.Provider(c.Request.Context(), req.PluginID)
		if err != nil {
			respondMarketError(c, err)
			return
		}
		priorRevision := ""
		if existing, err := mgr.Get(c.Request.Context(), req.PluginID); err == nil {
			priorRevision = existing.Revision
			if !existing.Provider.Known() && !req.ConfirmLegacyProvider {
				dto.ResponseError(c, 409, "发布者身份待确认，请先为旧安装补齐 provider 身份", nil)
				return
			}
			if existing.Provider.Known() && !existing.Provider.SameProvider(provider) {
				dto.ResponseError(c, 409, "相同插件 ID 属于不同发布者，不能替换原插件", nil)
				return
			}
			if req.Replace && (req.ExpectedRevision == "" || req.ExpectedRevision != existing.Revision) {
				dto.ResponseError(c, 409, "当前安装已变化，请刷新后重新确认替换", nil)
				return
			}
			if !req.Replace && existing.Version != req.Version {
				dto.ResponseError(c, 409, "插件已安装，请明确选择替换当前安装", nil)
				return
			}
		} else if !plugin_mgr.IsCode(err, plugin_mgr.CodeNotFound) {
			respondPluginRuntimeUnavailable(c, err)
			return
		} else if req.Replace {
			dto.ResponseError(c, 409, "原安装已变化，请刷新后重试", nil)
			return
		}

		attempt, err := deps.PluginMarketplace.BeginInstall(req.PluginID, req.Version, reqctx.GetTenantUUID(c.Request.Context()), req.Enable, priorRevision)
		if err != nil {
			respondMarketError(c, err)
			return
		}
		download, err := deps.PluginMarketplace.ResolveDownload(c.Request.Context(), req.PluginID, req.Version, reqctx.GetTenantUUID(c.Request.Context()), attempt)
		if err != nil {
			logMarketplaceReportError(c.Request.Context(), deps.PluginMarketplace.RecordOperationFailed(attempt, err, "POWERX_DOWNLOAD_FAILED"))
			respondMarketError(c, err)
			return
		}
		p, err := mgr.InstallFromURL(c.Request.Context(), download.SignedURL, download.SHA256, "", plugin_mgr.InstallOptions{
			Origin:   deps.PluginMarketplace.Origin(),
			Provider: provider, Force: req.Replace, ExpectedRevision: req.ExpectedRevision, AutoEnable: req.Enable, ConfirmLegacyProvider: req.ConfirmLegacyProvider,
			VerifyChecksum: true, ExpectedPluginID: req.PluginID, ExpectedVersion: req.Version, ExpectedSize: download.Size,
			DownloadVerified: func(_ context.Context, verified plugin_mgr.DownloadVerification) error {
				return deps.PluginMarketplace.RecordDownloaded(attempt, verified)
			},
			HostConfigSeed: seed, Metadata: plugin_mgr.InstallMetadata{Scope: "system"},
		})
		if err != nil {
			logMarketplaceReportError(c.Request.Context(), deps.PluginMarketplace.RecordOperationFailed(attempt, err, "POWERX_INSTALL_FAILED"))
			respondMarketError(c, err)
			return
		}
		logMarketplaceReportError(c.Request.Context(), deps.PluginMarketplace.RecordInstalled(attempt, p))
		finishPluginInstallation(c, deps, p, req.Enable)
		if req.Enable {
			if c.Writer.Status() < 400 {
				logMarketplaceReportError(c.Request.Context(), deps.PluginMarketplace.RecordEnabled(attempt))
			} else {
				logMarketplaceReportError(c.Request.Context(), deps.PluginMarketplace.RecordFailed(attempt, "POWERX_ENABLE_FAILED"))
			}
		}
	}
}

func logMarketplaceReportError(ctx context.Context, err error) {
	if err != nil {
		logger.WarnF(ctx, "Marketplace 安装结果未完成持久化，请核对宿主安装记录；不会重新执行安装")
	}
}
