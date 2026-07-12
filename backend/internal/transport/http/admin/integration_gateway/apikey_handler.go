package integration_gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	apikeycache "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeycache"
	apikeypermissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"

	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type APIKeyAdminHandler struct {
	db       *gorm.DB
	keys     *repo.IntegrationGatewayAPIKeyRepository
	perms    *repo.IntegrationGatewayAPIKeyPermissionRepository
	iamKeys  *iamrepo.APIKeyRepository
	profiles *iamrepo.APIKeyProfileRepository
	permRepo *iamrepo.PermissionRepository
	profPerm *iamrepo.APIKeyProfilePermissionRepository

	ensureTemplateMu     sync.Mutex
	templatesEnsuredOnce bool
}

func NewAPIKeyAdminHandler(db *gorm.DB) *APIKeyAdminHandler {
	return &APIKeyAdminHandler{
		db:       db,
		keys:     repo.NewIntegrationGatewayAPIKeyRepository(db),
		perms:    repo.NewIntegrationGatewayAPIKeyPermissionRepository(db),
		iamKeys:  iamrepo.NewAPIKeyRepository(db),
		profiles: iamrepo.NewAPIKeyProfileRepository(db),
		permRepo: iamrepo.NewPermissionRepository(db),
		profPerm: iamrepo.NewAPIKeyProfilePermissionRepository(db),
	}
}

type createAPIKeyRequest struct {
	TenantUUID  string `json:"tenant_uuid"`
	ProfileID   uint64 `json:"profile_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	ExpiresAt   string `json:"expires_at"`
}

type revokeAPIKeyRequest struct {
	TenantUUID string `json:"tenant_uuid"`
}

type rotateAPIKeyRequest struct {
	TenantUUID  string `json:"tenant_uuid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ExpiresAt   string `json:"expires_at"`
}

type setAPIKeyProfilePermissionsRequest struct {
	PermissionIDs []uint64 `json:"permission_ids"`
}

type apiKeyPermissionRequest struct {
	Scope           string `json:"scope"`
	Action          string `json:"action"`
	ResourceType    string `json:"resource_type"`
	ResourcePattern string `json:"resource_pattern"`
	PluginID        string `json:"plugin_id"`
	Effect          string `json:"effect"`
}

type apiKeyPermissionResponse struct {
	Scope           string `json:"scope"`
	Action          string `json:"action"`
	ResourceType    string `json:"resource_type"`
	ResourcePattern string `json:"resource_pattern"`
	PluginID        string `json:"plugin_id,omitempty"`
	Effect          string `json:"effect"`
}

type apiKeyResponse struct {
	KeyID       string                     `json:"key_id"`
	TenantUUID  string                     `json:"tenant_uuid"`
	ProfileID   uint64                     `json:"profile_id"`
	Name        string                     `json:"name"`
	Description string                     `json:"description,omitempty"`
	KeyPrefix   string                     `json:"key_prefix"`
	Status      string                     `json:"status"`
	ExpiresAt   *string                    `json:"expires_at,omitempty"`
	LastUsedAt  *string                    `json:"last_used_at,omitempty"`
	CreatedAt   string                     `json:"created_at"`
	UpdatedAt   string                     `json:"updated_at"`
	Permissions []apiKeyPermissionResponse `json:"permissions,omitempty"`
}

type apiKeyProfileResponse struct {
	ID            uint64  `json:"id"`
	TenantUUID    string  `json:"tenant_uuid"`
	OwnerMemberID *uint64 `json:"owner_member_id,omitempty"`
	Key           string  `json:"key"`
	Name          string  `json:"name"`
	Status        int16   `json:"status"`
}

type createAPIKeyProfileRequest struct {
	TenantUUID string `json:"tenant_uuid"`
	Key        string `json:"key"`
	Name       string `json:"name"`
}

type updateAPIKeyProfileRequest struct {
	TenantUUID string `json:"tenant_uuid"`
	Name       string `json:"name"`
	Status     *int16 `json:"status"`
}

func (h *APIKeyAdminHandler) CreateAPIKey(c *gin.Context) {
	var req createAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	profile, err := h.profiles.GetById(c.Request.Context(), req.ProfileID, nil)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key profile failed", err))
		return
	}
	if profile == nil || profile.Status != 1 || !strings.EqualFold(strings.TrimSpace(profile.TenantUUID), tenantUUID) {
		dto.RespondErrorFrom(c, dto.NewBadRequest("profile_id invalid or disabled", nil))
		return
	}

	expiresAt, err := parseOptionalTime(req.ExpiresAt)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("expires_at must be RFC3339", err))
		return
	}

	plain, keyHash, keyPrefix := generateAPIKeyMaterial()
	perms, err := h.resolveProfileAPIKeyPermissions(c.Request.Context(), req.ProfileID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("profile permissions invalid", err))
		return
	}
	actor := actorFromHeader(c)

	var created *models.IntegrationGatewayAPIKey
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		keyRepo := repo.NewIntegrationGatewayAPIKeyRepository(tx)
		permRepo := repo.NewIntegrationGatewayAPIKeyPermissionRepository(tx)
		iamKeyRepo := iamrepo.NewAPIKeyRepository(tx)

		item := &models.IntegrationGatewayAPIKey{
			TenantUUID:  tenantUUID,
			ProfileID:   req.ProfileID,
			Name:        strings.TrimSpace(req.Name),
			Description: strings.TrimSpace(req.Description),
			KeyPrefix:   keyPrefix,
			KeyHash:     keyHash,
			Status:      "active",
			ExpiresAt:   expiresAt,
			CreatedBy:   actor,
			UpdatedBy:   actor,
		}
		var e error
		created, e = keyRepo.Create(c.Request.Context(), item)
		if e != nil {
			return e
		}

		permissionModels := buildPermissionModels(created.UUID, perms)
		if e = permRepo.ReplaceAll(c.Request.Context(), created.UUID, permissionModels); e != nil {
			return e
		}

		e = iamKeyRepo.Create(c.Request.Context(), &modelsiam.APIKey{
			TenantUUID:  tenantUUID,
			ProfileID:   req.ProfileID,
			KeyHash:     keyHash,
			CreatedAtMs: time.Now().UnixMilli(),
		})
		if e != nil {
			return e
		}
		return nil
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("create api key failed", err))
		return
	}

	permResps := toPermissionResponses(buildPermissionModels(created.UUID, perms))
	resp := toAPIKeyResponse(*created, permResps)
	_ = apikeycache.InvalidateAll(c.Request.Context())
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{
		"api_key":   resp,
		"plain_key": plain,
	})
}

func (h *APIKeyAdminHandler) ListAPIKeys(c *gin.Context) {
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	items, total, err := h.keys.ListByTenant(c.Request.Context(), canonical, (page-1)*pageSize, pageSize)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list api keys failed", err))
		return
	}

	respItems := make([]apiKeyResponse, 0, len(items))
	for i := range items {
		perms, _ := h.perms.ListByAPIKeyUUID(c.Request.Context(), items[i].UUID)
		respItems = append(respItems, toAPIKeyResponse(items[i], toPermissionResponses(perms)))
	}
	dto.ResponseSuccess(c, dto.ListResponse{
		Items: respItems,
		Pagination: &dto.PaginationResponse{
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	})
}

func (h *APIKeyAdminHandler) GetAPIKey(c *gin.Context) {
	keyID, err := uuid.Parse(strings.TrimSpace(c.Param("key_id")))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid key_id", err))
		return
	}
	item, err := h.keys.GetByUUID(c.Request.Context(), keyID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("get api key failed", err))
		return
	}
	if item == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key not found", nil))
		return
	}
	canonical, err := h.resolveTenantScope(c, item.TenantUUID)
	if err != nil || !strings.EqualFold(strings.TrimSpace(item.TenantUUID), canonical) {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key not found", nil))
		return
	}
	perms, _ := h.perms.ListByAPIKeyUUID(c.Request.Context(), item.UUID)
	dto.ResponseSuccess(c, toAPIKeyResponse(*item, toPermissionResponses(perms)))
}

func (h *APIKeyAdminHandler) RevokeAPIKey(c *gin.Context) {
	keyID, err := uuid.Parse(strings.TrimSpace(c.Param("key_id")))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid key_id", err))
		return
	}
	var req revokeAPIKeyRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	item, err := h.keys.GetByUUID(c.Request.Context(), keyID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key failed", err))
		return
	}
	if item == nil || !strings.EqualFold(strings.TrimSpace(item.TenantUUID), canonical) {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key not found", nil))
		return
	}

	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		keyRepo := repo.NewIntegrationGatewayAPIKeyRepository(tx)
		iamKeyRepo := iamrepo.NewAPIKeyRepository(tx)
		if e := keyRepo.UpdateStatus(c.Request.Context(), item.UUID, "revoked", actorFromHeader(c)); e != nil {
			return e
		}
		if legacy, e := iamKeyRepo.FindByHash(c.Request.Context(), item.KeyHash); e == nil && legacy != nil {
			return iamKeyRepo.Revoke(c.Request.Context(), legacy.ID, time.Now().UnixMilli())
		}
		return nil
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("revoke api key failed", err))
		return
	}
	_ = apikeycache.InvalidateAll(c.Request.Context())
	dto.ResponseSuccess(c, gin.H{"status": "revoked", "key_id": keyID.String()})
}

func (h *APIKeyAdminHandler) DeleteAPIKey(c *gin.Context) {
	keyID, err := uuid.Parse(strings.TrimSpace(c.Param("key_id")))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid key_id", err))
		return
	}
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	item, err := h.keys.GetByUUID(c.Request.Context(), keyID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key failed", err))
		return
	}
	if item == nil || !strings.EqualFold(strings.TrimSpace(item.TenantUUID), canonical) {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key not found", nil))
		return
	}

	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		keyRepo := repo.NewIntegrationGatewayAPIKeyRepository(tx)
		iamKeyRepo := iamrepo.NewAPIKeyRepository(tx)
		if e := keyRepo.UpdateStatus(c.Request.Context(), item.UUID, "deleted", actorFromHeader(c)); e != nil {
			return e
		}
		if legacy, e := iamKeyRepo.FindByHash(c.Request.Context(), item.KeyHash); e == nil && legacy != nil {
			if e = iamKeyRepo.Revoke(c.Request.Context(), legacy.ID, time.Now().UnixMilli()); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("delete api key failed", err))
		return
	}
	_ = apikeycache.InvalidateAll(c.Request.Context())
	dto.ResponseSuccess(c, gin.H{"status": "deleted", "key_id": keyID.String()})
}

func (h *APIKeyAdminHandler) RotateAPIKey(c *gin.Context) {
	keyID, err := uuid.Parse(strings.TrimSpace(c.Param("key_id")))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid key_id", err))
		return
	}
	var req rotateAPIKeyRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	oldItem, err := h.keys.GetByUUID(c.Request.Context(), keyID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key failed", err))
		return
	}
	if oldItem == nil || !strings.EqualFold(strings.TrimSpace(oldItem.TenantUUID), canonical) {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key not found", nil))
		return
	}

	expiresAt, err := parseOptionalTime(req.ExpiresAt)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("expires_at must be RFC3339", err))
		return
	}
	plain, keyHash, keyPrefix := generateAPIKeyMaterial()
	actor := actorFromHeader(c)
	perms, err := h.resolveProfileAPIKeyPermissions(c.Request.Context(), oldItem.ProfileID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("profile permissions invalid", err))
		return
	}

	var created *models.IntegrationGatewayAPIKey
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		keyRepo := repo.NewIntegrationGatewayAPIKeyRepository(tx)
		permRepo := repo.NewIntegrationGatewayAPIKeyPermissionRepository(tx)
		iamKeyRepo := iamrepo.NewAPIKeyRepository(tx)

		if e := keyRepo.UpdateStatus(c.Request.Context(), oldItem.UUID, "revoked", actor); e != nil {
			return e
		}
		if legacy, e := iamKeyRepo.FindByHash(c.Request.Context(), oldItem.KeyHash); e == nil && legacy != nil {
			if e = iamKeyRepo.Revoke(c.Request.Context(), legacy.ID, time.Now().UnixMilli()); e != nil {
				return e
			}
		}

		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = oldItem.Name
		}
		desc := strings.TrimSpace(req.Description)
		if desc == "" {
			desc = oldItem.Description
		}
		item := &models.IntegrationGatewayAPIKey{
			TenantUUID:  oldItem.TenantUUID,
			ProfileID:   oldItem.ProfileID,
			Name:        name,
			Description: desc,
			KeyPrefix:   keyPrefix,
			KeyHash:     keyHash,
			Status:      "active",
			ExpiresAt:   expiresAt,
			CreatedBy:   actor,
			UpdatedBy:   actor,
		}
		var e error
		created, e = keyRepo.Create(c.Request.Context(), item)
		if e != nil {
			return e
		}
		if e = permRepo.ReplaceAll(c.Request.Context(), created.UUID, buildPermissionModels(created.UUID, perms)); e != nil {
			return e
		}
		if e = iamKeyRepo.Create(c.Request.Context(), &modelsiam.APIKey{
			TenantUUID:  created.TenantUUID,
			ProfileID:   created.ProfileID,
			KeyHash:     created.KeyHash,
			CreatedAtMs: time.Now().UnixMilli(),
		}); e != nil {
			return e
		}
		return nil
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("rotate api key failed", err))
		return
	}

	permModels := buildPermissionModels(created.UUID, perms)
	_ = apikeycache.InvalidateAll(c.Request.Context())
	dto.ResponseSuccess(c, gin.H{
		"api_key":   toAPIKeyResponse(*created, toPermissionResponses(permModels)),
		"plain_key": plain,
		"rotated":   keyID.String(),
	})
}

func (h *APIKeyAdminHandler) ListAPIKeyProfiles(c *gin.Context) {
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	items, err := h.profiles.ListByTenant(c.Request.Context(), canonical)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list api key profiles failed", err))
		return
	}
	resp := make([]apiKeyProfileResponse, 0, len(items))
	for i := range items {
		resp = append(resp, apiKeyProfileResponse{
			ID:            items[i].ID,
			TenantUUID:    items[i].TenantUUID,
			OwnerMemberID: items[i].OwnerMemberID,
			Key:           items[i].Key,
			Name:          items[i].Name,
			Status:        items[i].Status,
		})
	}
	dto.ResponseSuccess(c, gin.H{"items": resp})
}

func (h *APIKeyAdminHandler) CreateAPIKeyProfile(c *gin.Context) {
	var req createAPIKeyProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var ownerMemberID *uint64
	if memberID := reqctx.GetMemberID(c.Request.Context()); memberID > 0 {
		value := memberID
		ownerMemberID = &value
	}

	const defaultAPIKeyProfilePrefix = "integration.default."
	defaultAPIKeyProfileKey := apikeypermissions.DefaultAPIKeyProfileKey
	defaultAPIKeyProfileName := apikeypermissions.DefaultAPIKeyProfileName
	accountKey := strings.TrimSpace(req.Key)
	if accountKey == "" {
		accountKey = defaultAPIKeyProfileKey
	}
	accountName := strings.TrimSpace(req.Name)
	if accountName == "" {
		accountName = defaultAPIKeyProfileName
	}

	if accountKey == defaultAPIKeyProfileKey {
		items, listErr := h.profiles.ListByTenant(c.Request.Context(), canonical)
		if listErr != nil {
			dto.RespondErrorFrom(c, dto.NewInternal("list api key profiles failed", listErr))
			return
		}
		for i := range items {
			currentKey := strings.TrimSpace(items[i].Key)
			if currentKey != defaultAPIKeyProfileKey && !strings.HasPrefix(currentKey, defaultAPIKeyProfilePrefix) {
				continue
			}
			changed := false
			if items[i].Status != 1 {
				items[i].Status = 1
				changed = true
			}
			if accountName != "" && items[i].Name != accountName {
				items[i].Name = accountName
				changed = true
			}
			if items[i].OwnerMemberID == nil && ownerMemberID != nil && *ownerMemberID > 0 {
				value := *ownerMemberID
				items[i].OwnerMemberID = &value
				changed = true
			}
			if changed {
				updated, updateErr := h.profiles.Update(c.Request.Context(), &items[i])
				if updateErr != nil {
					dto.RespondErrorFrom(c, dto.NewInternal("enable api key profile failed", updateErr))
					return
				}
				dto.ResponseSuccess(c, apiKeyProfileResponse{
					ID:            updated.ID,
					TenantUUID:    updated.TenantUUID,
					OwnerMemberID: updated.OwnerMemberID,
					Key:           updated.Key,
					Name:          updated.Name,
					Status:        updated.Status,
				})
				_ = apikeycache.InvalidateAll(c.Request.Context())
				return
			}
			dto.ResponseSuccess(c, apiKeyProfileResponse{
				ID:            items[i].ID,
				TenantUUID:    items[i].TenantUUID,
				OwnerMemberID: items[i].OwnerMemberID,
				Key:           items[i].Key,
				Name:          items[i].Name,
				Status:        items[i].Status,
			})
			return
		}
	}

	existed, err := h.profiles.FindByKey(c.Request.Context(), canonical, accountKey)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key profile failed", err))
		return
	}
	if existed != nil {
		if existed.Status != 1 {
			existed.Status = 1
			existed.Name = accountName
			if existed.OwnerMemberID == nil && ownerMemberID != nil && *ownerMemberID > 0 {
				value := *ownerMemberID
				existed.OwnerMemberID = &value
			}
			updated, updateErr := h.profiles.Update(c.Request.Context(), existed)
			if updateErr != nil {
				dto.RespondErrorFrom(c, dto.NewInternal("enable api key profile failed", updateErr))
				return
			}
			dto.ResponseSuccessWithStatus(c, http.StatusCreated, apiKeyProfileResponse{
				ID:            updated.ID,
				TenantUUID:    updated.TenantUUID,
				OwnerMemberID: updated.OwnerMemberID,
				Key:           updated.Key,
				Name:          updated.Name,
				Status:        updated.Status,
			})
			_ = apikeycache.InvalidateAll(c.Request.Context())
			return
		}
		dto.ResponseSuccess(c, apiKeyProfileResponse{
			ID:            existed.ID,
			TenantUUID:    existed.TenantUUID,
			OwnerMemberID: existed.OwnerMemberID,
			Key:           existed.Key,
			Name:          existed.Name,
			Status:        existed.Status,
		})
		return
	}

	created, err := h.profiles.Create(c.Request.Context(), &modelsiam.APIKeyProfile{
		TenantUUID:    canonical,
		OwnerMemberID: ownerMemberID,
		Key:           accountKey,
		Name:          accountName,
		Status:        1,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("create api key profile failed", err))
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, apiKeyProfileResponse{
		ID:            created.ID,
		TenantUUID:    created.TenantUUID,
		OwnerMemberID: created.OwnerMemberID,
		Key:           created.Key,
		Name:          created.Name,
		Status:        created.Status,
	})
	_ = apikeycache.InvalidateAll(c.Request.Context())
}

func (h *APIKeyAdminHandler) UpdateAPIKeyProfile(c *gin.Context) {
	var req updateAPIKeyProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}

	item, err := h.resolveAPIKeyProfile(c.Request.Context(), canonical, c.Param("profile_id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key profile failed", err))
		return
	}
	if item == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key profile not found", nil))
		return
	}

	updated := false
	trimmedName := strings.TrimSpace(req.Name)
	if trimmedName != "" && trimmedName != item.Name {
		item.Name = trimmedName
		updated = true
	}
	if req.Status != nil {
		if *req.Status != 0 && *req.Status != 1 {
			dto.RespondErrorFrom(c, dto.NewBadRequest("status must be 0 or 1", nil))
			return
		}
		if item.Status != *req.Status {
			item.Status = *req.Status
			updated = true
		}
	}
	if !updated {
		dto.ResponseSuccess(c, apiKeyProfileResponse{
			ID:            item.ID,
			TenantUUID:    item.TenantUUID,
			OwnerMemberID: item.OwnerMemberID,
			Key:           item.Key,
			Name:          item.Name,
			Status:        item.Status,
		})
		return
	}

	result, err := h.profiles.Update(c.Request.Context(), item)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("update api key profile failed", err))
		return
	}
	_ = apikeycache.InvalidateAll(c.Request.Context())
	dto.ResponseSuccess(c, apiKeyProfileResponse{
		ID:            result.ID,
		TenantUUID:    result.TenantUUID,
		OwnerMemberID: result.OwnerMemberID,
		Key:           result.Key,
		Name:          result.Name,
		Status:        result.Status,
	})
}

type apiKeyPermissionCatalogItem struct {
	ID          uint64         `json:"id"`
	Module      string         `json:"module"`
	Resource    string         `json:"resource"`
	Action      string         `json:"action"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Meta        map[string]any `json:"meta,omitempty"`
}

func (h *APIKeyAdminHandler) ListAPIKeyPermissionCatalog(c *gin.Context) {
	if err := h.ensureAPIKeyPermissionTemplates(c.Request.Context()); err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("ensure api key permissions failed", err))
		return
	}
	rows, _, err := h.permRepo.List(c.Request.Context(), map[string]string{
		"status":        string(modelsiam.PermissionStatusActive),
		"allow_api_key": "true",
	}, 0, 1000, "module ASC, resource ASC, action ASC")
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list permissions failed", err))
		return
	}
	items := make([]apiKeyPermissionCatalogItem, 0, len(rows))
	for i := range rows {
		meta := parsePermissionMeta(rows[i].Meta)
		if _, ok := toAPIKeyPermissionFromPermission(rows[i]); !ok {
			continue
		}
		items = append(items, apiKeyPermissionCatalogItem{
			ID:          rows[i].ID,
			Module:      rows[i].Module,
			Resource:    rows[i].Resource,
			Action:      rows[i].Action,
			Description: rows[i].Description,
			Status:      string(rows[i].Status),
			Meta:        meta,
		})
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *APIKeyAdminHandler) GetPermissionsByAPIKey(c *gin.Context) {
	canonical, err := h.resolveTenantScope(c, c.Query("tenant_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	plainKey := strings.TrimSpace(c.Query("apikey"))
	if plainKey == "" {
		dto.RespondErrorFrom(c, dto.NewBadRequest("apikey is required", nil))
		return
	}
	sum := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(sum[:])

	apiKey, err := h.keys.FindActiveByHash(c.Request.Context(), canonical, keyHash)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key failed", err))
		return
	}
	if apiKey == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key not found", nil))
		return
	}
	perms, err := h.perms.ListByAPIKeyUUID(c.Request.Context(), apiKey.UUID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list api key permissions failed", err))
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"tenant_uuid":  canonical,
		"apikey_found": true,
		"key_id":       apiKey.UUID.String(),
		"profile_id":   apiKey.ProfileID,
		"key_name":     apiKey.Name,
		"key_prefix":   apiKey.KeyPrefix,
		"status":       apiKey.Status,
		"permissions":  toPermissionResponses(perms),
		"count":        len(perms),
	})
}

func (h *APIKeyAdminHandler) GetAPIKeyProfilePermissions(c *gin.Context) {
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	profile, err := h.resolveAPIKeyProfile(c.Request.Context(), canonical, c.Param("profile_id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key profile failed", err))
		return
	}
	if profile == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key profile not found", nil))
		return
	}
	profileID := profile.ID
	permissionIDs, err := h.profPerm.ListPermissionIDsOfProfile(c.Request.Context(), profileID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list profile permissions failed", err))
		return
	}
	permissionRows, err := h.permRepo.FindByIDs(c.Request.Context(), permissionIDs)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load permissions failed", err))
		return
	}
	items := make([]apiKeyPermissionCatalogItem, 0, len(permissionRows))
	for i := range permissionRows {
		if permissionRows[i] == nil {
			continue
		}
		items = append(items, apiKeyPermissionCatalogItem{
			ID:          permissionRows[i].ID,
			Module:      permissionRows[i].Module,
			Resource:    permissionRows[i].Resource,
			Action:      permissionRows[i].Action,
			Description: permissionRows[i].Description,
			Status:      string(permissionRows[i].Status),
			Meta:        parsePermissionMeta(permissionRows[i].Meta),
		})
	}
	dto.ResponseSuccess(c, gin.H{
		"profile_id":      profileID,
		"permission_ids":  permissionIDs,
		"permission_rows": items,
	})
}

func (h *APIKeyAdminHandler) SetAPIKeyProfilePermissions(c *gin.Context) {
	var req setAPIKeyProfilePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	profile, err := h.resolveAPIKeyProfile(c.Request.Context(), canonical, c.Param("profile_id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key profile failed", err))
		return
	}
	if profile == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key profile not found", nil))
		return
	}
	profileID := profile.ID
	if err := h.ensureAPIKeyPermissionTemplates(c.Request.Context()); err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("ensure api key permissions failed", err))
		return
	}

	wantIDs := uniqueUint64(req.PermissionIDs)
	permissionRows, err := h.permRepo.FindByIDs(c.Request.Context(), wantIDs)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load permissions failed", err))
		return
	}
	validMap := make(map[uint64]struct{}, len(permissionRows))
	for i := range permissionRows {
		if permissionRows[i] == nil || permissionRows[i].Status != modelsiam.PermissionStatusActive || !permissionRows[i].AllowAPIKey {
			continue
		}
		if _, ok := toAPIKeyPermissionFromPermission(*permissionRows[i]); !ok {
			continue
		}
		validMap[permissionRows[i].ID] = struct{}{}
	}
	validIDs := make([]uint64, 0, len(validMap))
	invalidIDs := make([]uint64, 0)
	for _, id := range wantIDs {
		if _, ok := validMap[id]; ok {
			validIDs = append(validIDs, id)
		} else {
			invalidIDs = append(invalidIDs, id)
		}
	}
	sort.Slice(validIDs, func(i, j int) bool { return validIDs[i] < validIDs[j] })
	sort.Slice(invalidIDs, func(i, j int) bool { return invalidIDs[i] < invalidIDs[j] })
	if len(invalidIDs) > 0 {
		dto.RespondErrorFrom(c, dto.WithDetails(
			dto.NewBadRequest("contains invalid permission_ids", nil),
			map[string]interface{}{"invalid_ids": invalidIDs},
		))
		return
	}

	currentIDs, err := h.profPerm.ListPermissionIDsOfProfile(c.Request.Context(), profileID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list current profile permissions failed", err))
		return
	}
	toAdd, toRemove := diffUint64(currentIDs, validIDs)
	syncedKeys := 0
	syncedPermissions := 0
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		profPermRepo := iamrepo.NewAPIKeyProfilePermissionRepository(tx)
		if err := profPermRepo.RevokeByIDsTx(tx, profileID, toRemove); err != nil {
			return err
		}
		if err := profPermRepo.GrantByIDsTx(tx, profileID, toAdd); err != nil {
			return err
		}
		syncedKeys, syncedPermissions, err = h.syncActiveAPIKeySnapshotsForProfileTx(c.Request.Context(), tx, canonical, profileID)
		return err
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("save profile permissions failed", err))
		return
	}
	logger.InfoF(
		c.Request.Context(),
		"[integration_gateway.apikey] profile permissions synced tenant=%s profile_id=%d active_keys=%d snapshot_permissions=%d",
		canonical,
		profileID,
		syncedKeys,
		syncedPermissions,
	)

	_ = apikeycache.InvalidateAll(c.Request.Context())
	dto.ResponseSuccess(c, gin.H{
		"profile_id":     profileID,
		"profile_key":    profile.Key,
		"permission_ids": validIDs,
		"added":          toAdd,
		"removed":        toRemove,
		"synced_keys":    syncedKeys,
		"synced_perms":   syncedPermissions,
	})
}

func (h *APIKeyAdminHandler) AppendAPIKeyProfilePermissions(c *gin.Context) {
	var req setAPIKeyProfilePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	canonical, err := h.resolveTenantScope(c, "")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	profile, err := h.resolveAPIKeyProfile(c.Request.Context(), canonical, c.Param("profile_id"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load api key profile failed", err))
		return
	}
	if profile == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("api key profile not found", nil))
		return
	}
	if err := h.ensureAPIKeyPermissionTemplates(c.Request.Context()); err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("ensure api key permissions failed", err))
		return
	}

	wantIDs := uniqueUint64(req.PermissionIDs)
	permissionRows, err := h.permRepo.FindByIDs(c.Request.Context(), wantIDs)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("load permissions failed", err))
		return
	}
	validMap := make(map[uint64]struct{}, len(permissionRows))
	for i := range permissionRows {
		if permissionRows[i] == nil || permissionRows[i].Status != modelsiam.PermissionStatusActive || !permissionRows[i].AllowAPIKey {
			continue
		}
		if _, ok := toAPIKeyPermissionFromPermission(*permissionRows[i]); !ok {
			continue
		}
		validMap[permissionRows[i].ID] = struct{}{}
	}
	validIDs := make([]uint64, 0, len(validMap))
	invalidIDs := make([]uint64, 0)
	for _, id := range wantIDs {
		if _, ok := validMap[id]; ok {
			validIDs = append(validIDs, id)
		} else {
			invalidIDs = append(invalidIDs, id)
		}
	}
	sort.Slice(validIDs, func(i, j int) bool { return validIDs[i] < validIDs[j] })
	sort.Slice(invalidIDs, func(i, j int) bool { return invalidIDs[i] < invalidIDs[j] })
	if len(invalidIDs) > 0 {
		dto.RespondErrorFrom(c, dto.WithDetails(
			dto.NewBadRequest("contains invalid permission_ids", nil),
			map[string]interface{}{"invalid_ids": invalidIDs},
		))
		return
	}

	currentIDs, err := h.profPerm.ListPermissionIDsOfProfile(c.Request.Context(), profile.ID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list current profile permissions failed", err))
		return
	}
	toAdd, _ := diffUint64(currentIDs, validIDs)
	syncedKeys := 0
	syncedPermissions := 0
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		profPermRepo := iamrepo.NewAPIKeyProfilePermissionRepository(tx)
		if err := profPermRepo.GrantByIDsTx(tx, profile.ID, toAdd); err != nil {
			return err
		}
		syncedKeys, syncedPermissions, err = h.syncActiveAPIKeySnapshotsForProfileTx(c.Request.Context(), tx, canonical, profile.ID)
		return err
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("append profile permissions failed", err))
		return
	}

	_ = apikeycache.InvalidateAll(c.Request.Context())
	mergedIDs := mergeUint64(currentIDs, validIDs)
	dto.ResponseSuccess(c, gin.H{
		"profile_id":     profile.ID,
		"profile_key":    profile.Key,
		"permission_ids": mergedIDs,
		"added":          toAdd,
		"removed":        []uint64{},
		"synced_keys":    syncedKeys,
		"synced_perms":   syncedPermissions,
	})
}

func (h *APIKeyAdminHandler) resolveAPIKeyProfile(ctx context.Context, tenantUUID string, profileOrKey string) (*modelsiam.APIKeyProfile, error) {
	raw := strings.TrimSpace(profileOrKey)
	if raw == "" {
		return nil, nil
	}
	if profileID, err := strconv.ParseUint(raw, 10, 64); err == nil && profileID > 0 {
		item, getErr := h.profiles.GetById(ctx, profileID, nil)
		if getErr != nil {
			return nil, getErr
		}
		if item == nil || !strings.EqualFold(strings.TrimSpace(item.TenantUUID), strings.TrimSpace(tenantUUID)) {
			return nil, nil
		}
		return item, nil
	}
	item, err := h.profiles.FindByKey(ctx, strings.TrimSpace(tenantUUID), raw)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if item == nil || !strings.EqualFold(strings.TrimSpace(item.TenantUUID), strings.TrimSpace(tenantUUID)) {
		return nil, nil
	}
	return item, nil
}

func (h *APIKeyAdminHandler) resolveProfileAPIKeyPermissions(ctx context.Context, profileID uint64) ([]apiKeyPermissionRequest, error) {
	permissionIDs, err := h.profPerm.ListPermissionIDsOfProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if len(permissionIDs) == 0 {
		return []apiKeyPermissionRequest{}, nil
	}
	permissionRows, err := h.permRepo.FindByIDs(ctx, permissionIDs)
	if err != nil {
		return nil, err
	}
	out := make([]apiKeyPermissionRequest, 0, len(permissionRows))
	for i := range permissionRows {
		if permissionRows[i] == nil || permissionRows[i].Status != modelsiam.PermissionStatusActive || !permissionRows[i].AllowAPIKey {
			continue
		}
		item, ok := toAPIKeyPermissionFromPermission(*permissionRows[i])
		if !ok {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (h *APIKeyAdminHandler) syncActiveAPIKeySnapshotsForProfileTx(ctx context.Context, tx *gorm.DB, tenantUUID string, profileID uint64) (int, int, error) {
	if tx == nil {
		return 0, 0, fmt.Errorf("tx is nil")
	}
	profPermRepo := iamrepo.NewAPIKeyProfilePermissionRepository(tx)
	permRepo := iamrepo.NewPermissionRepository(tx)
	keyRepo := repo.NewIntegrationGatewayAPIKeyRepository(tx)
	keyPermRepo := repo.NewIntegrationGatewayAPIKeyPermissionRepository(tx)

	permissionIDs, err := profPermRepo.ListPermissionIDsOfProfile(ctx, profileID)
	if err != nil {
		return 0, 0, err
	}
	permissionRows, err := permRepo.FindByIDs(ctx, permissionIDs)
	if err != nil {
		return 0, 0, err
	}
	permissionRequests := make([]apiKeyPermissionRequest, 0, len(permissionRows))
	for i := range permissionRows {
		if permissionRows[i] == nil || permissionRows[i].Status != modelsiam.PermissionStatusActive || !permissionRows[i].AllowAPIKey {
			continue
		}
		item, ok := toAPIKeyPermissionFromPermission(*permissionRows[i])
		if !ok {
			continue
		}
		permissionRequests = append(permissionRequests, item)
	}

	keys, err := keyRepo.ListActiveByProfile(ctx, tenantUUID, profileID)
	if err != nil {
		return 0, 0, err
	}
	for i := range keys {
		if err := keyPermRepo.ReplaceAll(ctx, keys[i].UUID, buildPermissionModels(keys[i].UUID, permissionRequests)); err != nil {
			return 0, 0, err
		}
	}
	return len(keys), len(permissionRequests), nil
}

func (h *APIKeyAdminHandler) ensureAPIKeyPermissionTemplates(ctx context.Context) error {
	h.ensureTemplateMu.Lock()
	defer h.ensureTemplateMu.Unlock()
	if h.templatesEnsuredOnce {
		return nil
	}
	if err := apikeypermissions.EnsureTemplatePermissions(ctx, h.permRepo); err != nil {
		return err
	}
	h.templatesEnsuredOnce = true
	logger.InfoF(ctx, "[integration_gateway.apikey] ensure permission templates initialized once")
	return nil
}

type apiKeyPermissionMeta struct {
	Scope           string
	Action          string
	ResourceType    string
	ResourcePattern string
	PluginID        string
	Effect          string
}

func parsePermissionMeta(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func extractAPIKeyPermissionMeta(meta map[string]any) (apiKeyPermissionMeta, bool) {
	raw, ok := meta["api_key"]
	if !ok {
		return apiKeyPermissionMeta{}, false
	}
	data, ok := raw.(map[string]any)
	if !ok {
		return apiKeyPermissionMeta{}, false
	}
	item := apiKeyPermissionMeta{
		Scope:           strings.TrimSpace(anyToString(data["scope"])),
		Action:          strings.TrimSpace(anyToString(data["action"])),
		ResourceType:    strings.TrimSpace(anyToString(data["resource_type"])),
		ResourcePattern: strings.TrimSpace(anyToString(data["resource_pattern"])),
		PluginID:        strings.TrimSpace(anyToString(data["plugin_id"])),
		Effect:          strings.TrimSpace(anyToString(data["effect"])),
	}
	if item.Scope == "" || item.Action == "" || item.ResourceType == "" || item.ResourcePattern == "" {
		return apiKeyPermissionMeta{}, false
	}
	if item.Effect == "" {
		item.Effect = "allow"
	}
	return item, true
}

func toAPIKeyPermissionFromPermission(permission modelsiam.Permission) (apiKeyPermissionRequest, bool) {
	resolved, ok := apikeypermissions.ResolvePermission(permission)
	if !ok {
		return apiKeyPermissionRequest{}, false
	}
	return apiKeyPermissionRequest{
		Scope:           resolved.Scope,
		Action:          resolved.Action,
		ResourceType:    resolved.ResourceType,
		ResourcePattern: resolved.ResourcePattern,
		PluginID:        resolved.PluginID,
		Effect:          resolved.Effect,
	}, true
}

func diffUint64(oldIDs []uint64, newIDs []uint64) (toAdd []uint64, toRemove []uint64) {
	oldSet := make(map[uint64]struct{}, len(oldIDs))
	newSet := make(map[uint64]struct{}, len(newIDs))
	for _, id := range oldIDs {
		oldSet[id] = struct{}{}
	}
	for _, id := range newIDs {
		newSet[id] = struct{}{}
	}
	for _, id := range newIDs {
		if _, ok := oldSet[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}
	for _, id := range oldIDs {
		if _, ok := newSet[id]; !ok {
			toRemove = append(toRemove, id)
		}
	}
	sort.Slice(toAdd, func(i, j int) bool { return toAdd[i] < toAdd[j] })
	sort.Slice(toRemove, func(i, j int) bool { return toRemove[i] < toRemove[j] })
	return
}

func mergeUint64(a []uint64, b []uint64) []uint64 {
	set := make(map[uint64]struct{}, len(a)+len(b))
	out := make([]uint64, 0, len(a)+len(b))
	for _, id := range a {
		if id == 0 {
			continue
		}
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range b {
		if id == 0 {
			continue
		}
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func uniqueUint64(items []uint64) []uint64 {
	if len(items) == 0 {
		return nil
	}
	set := make(map[uint64]struct{}, len(items))
	out := make([]uint64, 0, len(items))
	for _, id := range items {
		if id == 0 {
			continue
		}
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func generateAPIKeyMaterial() (plain string, hash string, prefix string) {
	plain = "pxk_" + utils.RandomString(48)
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	if len(plain) > 12 {
		prefix = plain[:12]
	} else {
		prefix = plain
	}
	return plain, hash, prefix
}

func parseOptionalTime(raw string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	v, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	out := v.UTC()
	return &out, nil
}

func normalizePermissionInputs(items []apiKeyPermissionRequest) []apiKeyPermissionRequest {
	if len(items) == 0 {
		return nil
	}
	out := make([]apiKeyPermissionRequest, 0, len(items))
	for i := range items {
		scope := strings.TrimSpace(items[i].Scope)
		action := strings.TrimSpace(items[i].Action)
		resourceType := strings.TrimSpace(items[i].ResourceType)
		resourcePattern := strings.TrimSpace(items[i].ResourcePattern)
		if scope == "" || action == "" || resourceType == "" || resourcePattern == "" {
			continue
		}
		effect := strings.ToLower(strings.TrimSpace(items[i].Effect))
		if effect == "" {
			effect = "allow"
		}
		out = append(out, apiKeyPermissionRequest{
			Scope:           scope,
			Action:          action,
			ResourceType:    resourceType,
			ResourcePattern: resourcePattern,
			PluginID:        strings.TrimSpace(items[i].PluginID),
			Effect:          effect,
		})
	}
	return out
}

func buildPermissionModels(keyUUID uuid.UUID, items []apiKeyPermissionRequest) []models.IntegrationGatewayAPIKeyPermission {
	out := make([]models.IntegrationGatewayAPIKeyPermission, 0, len(items))
	for i := range items {
		out = append(out, models.IntegrationGatewayAPIKeyPermission{
			APIKeyUUID:      keyUUID,
			Scope:           items[i].Scope,
			Action:          items[i].Action,
			ResourceType:    items[i].ResourceType,
			ResourcePattern: items[i].ResourcePattern,
			PluginID:        items[i].PluginID,
			Effect:          items[i].Effect,
		})
	}
	return out
}

func toPermissionResponses(items []models.IntegrationGatewayAPIKeyPermission) []apiKeyPermissionResponse {
	out := make([]apiKeyPermissionResponse, 0, len(items))
	for i := range items {
		out = append(out, apiKeyPermissionResponse{
			Scope:           items[i].Scope,
			Action:          items[i].Action,
			ResourceType:    items[i].ResourceType,
			ResourcePattern: items[i].ResourcePattern,
			PluginID:        items[i].PluginID,
			Effect:          items[i].Effect,
		})
	}
	return out
}

func toAPIKeyResponse(item models.IntegrationGatewayAPIKey, perms []apiKeyPermissionResponse) apiKeyResponse {
	resp := apiKeyResponse{
		KeyID:       item.UUID.String(),
		TenantUUID:  item.TenantUUID,
		ProfileID:   item.ProfileID,
		Name:        item.Name,
		Description: item.Description,
		KeyPrefix:   item.KeyPrefix,
		Status:      item.Status,
		CreatedAt:   item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   item.UpdatedAt.Format(time.RFC3339),
		Permissions: perms,
	}
	if item.ExpiresAt != nil {
		v := item.ExpiresAt.UTC().Format(time.RFC3339)
		resp.ExpiresAt = &v
	}
	if item.LastUsedAt != nil {
		v := item.LastUsedAt.UTC().Format(time.RFC3339)
		resp.LastUsedAt = &v
	}
	return resp
}

func (h *APIKeyAdminHandler) resolveTenantScope(c *gin.Context, raw string) (string, error) {
	currentTenantRaw, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		return "", dto.NewUnauthorized("tenant context missing", err)
	}
	currentTenant, err := reqctx.CanonicalTenantUUID(strings.TrimSpace(currentTenantRaw))
	if err != nil {
		return "", dto.NewUnauthorized("tenant context invalid", err)
	}
	_ = raw
	return currentTenant, nil
}
