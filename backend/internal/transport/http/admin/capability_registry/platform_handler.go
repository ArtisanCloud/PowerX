package capability_registry

import (
	"sort"
	"strings"
	"time"

	capabilitycatalog "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

const platformCapabilitiesPageSize = 200

type platformCapabilityHandler struct {
	svc *capabilitycatalog.RegistryService
}

func newPlatformHandler(svc *capabilitycatalog.RegistryService) *platformCapabilityHandler {
	if svc == nil {
		return nil
	}
	return &platformCapabilityHandler{svc: svc}
}

func (h *platformCapabilityHandler) ListModules(c *gin.Context) {
	page := parsePositiveInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parsePositiveInt(c.DefaultQuery("page_size", "20"), 20)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	h.handleResponse(c, capability_registrydto.NormalizePlatformModuleKey(c.Query("module")), false, page, pageSize)
}

func (h *platformCapabilityHandler) GetModule(c *gin.Context) {
	h.handleResponse(c, capability_registrydto.NormalizePlatformModuleKey(c.Param("moduleKey")), true, 1, 1)
}

func (h *platformCapabilityHandler) handleResponse(c *gin.Context, moduleFilter string, single bool, page int, pageSize int) {
	if !reqctx.IsRoot(c.Request.Context()) {
		capability_registrydto.RespondError(c, capability_registrydto.ErrCapabilityForbidden.WithHint("仅 Root 管理员可查看平台能力"), nil)
		return
	}
	modules, totalCapabilities, err := h.loadModules(c, moduleFilter)
	if err != nil {
		capability_registrydto.RespondError(c, capability_registrydto.ErrInternal, err)
		return
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	if single {
		if len(modules) == 0 {
			capability_registrydto.RespondError(c, capability_registrydto.ErrNotFound.WithHint("指定模块不存在"), nil)
			return
		}
		dto.ResponseSuccess(c, gin.H{
			"generated_at": generatedAt,
			"module":       modules[0],
		})
		return
	}

	totalModules := len(modules)
	if pageSize <= 0 {
		pageSize = totalModules
	}
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	if start > totalModules {
		start = totalModules
	}
	end := start + pageSize
	if end > totalModules {
		end = totalModules
	}
	pagedModules := modules[start:end]

	dto.ResponseSuccess(c, gin.H{
		"generated_at":       generatedAt,
		"total_modules":      totalModules,
		"total_capabilities": totalCapabilities,
		"page":               page,
		"page_size":          pageSize,
		"modules":            pagedModules,
	})
}

func (h *platformCapabilityHandler) loadModules(c *gin.Context, moduleFilter string) ([]capability_registrydto.PlatformCapabilityModuleDTO, int, error) {
	ctx := c.Request.Context()
	grouped := map[string]*capability_registrydto.PlatformCapabilityModuleDTO{}
	moduleChannels := map[string]map[string]struct{}{}
	totalCapabilities := 0

	offset := 0
	var expectedTotal int64 = -1
	for {
		includeTotal := offset == 0
		opts := capabilitycatalog.CapabilityListOptions{
			Source:       capabilitycatalog.CapabilitySourceCoreX,
			Limit:        platformCapabilitiesPageSize,
			Offset:       offset,
			IncludeTotal: includeTotal,
		}
		views, total, err := h.svc.ListCapabilities(ctx, opts)
		if err != nil {
			return nil, 0, err
		}
		if includeTotal {
			expectedTotal = total
		}
		if len(views) == 0 {
			break
		}
		for _, view := range views {
			capDTO := capability_registrydto.PlatformCapabilityToDTO(view.Record)
			if moduleFilter != "" && capDTO.Module != moduleFilter {
				continue
			}
			moduleKey := capDTO.Module
			if moduleKey == "" {
				moduleKey = "corex"
			}
			moduleDTO, exists := grouped[moduleKey]
			if !exists {
				module := capability_registrydto.NewPlatformCapabilityModuleDTO(moduleKey)
				moduleDTO = &module
				grouped[moduleKey] = moduleDTO
			}
			moduleDTO.Capabilities = append(moduleDTO.Capabilities, capDTO)
			totalCapabilities++

			channelSet, ok := moduleChannels[moduleKey]
			if !ok {
				channelSet = map[string]struct{}{}
				moduleChannels[moduleKey] = channelSet
			}
			for _, binding := range capDTO.Protocols {
				channel := strings.TrimSpace(binding.Channel)
				if channel == "" {
					continue
				}
				channelSet[channel] = struct{}{}
			}
		}
		offset += len(views)
		if expectedTotal >= 0 && int64(offset) >= expectedTotal {
			break
		}
	}

	if moduleFilter != "" {
		if moduleDTO, ok := grouped[moduleFilter]; ok {
			moduleDTO.CapabilityCount = len(moduleDTO.Capabilities)
			moduleDTO.ProtocolChannels = deduplicateAndSort(moduleChannels[moduleFilter])
			sortCapabilities(moduleDTO.Capabilities)
			return []capability_registrydto.PlatformCapabilityModuleDTO{*moduleDTO}, len(moduleDTO.Capabilities), nil
		}
		return nil, 0, nil
	}

	modules := make([]capability_registrydto.PlatformCapabilityModuleDTO, 0, len(grouped))
	for key, moduleDTO := range grouped {
		moduleDTO.CapabilityCount = len(moduleDTO.Capabilities)
		moduleDTO.ProtocolChannels = deduplicateAndSort(moduleChannels[key])
		sortCapabilities(moduleDTO.Capabilities)
		modules = append(modules, *moduleDTO)
	}
	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Module < modules[j].Module
	})

	return modules, totalCapabilities, nil
}

func deduplicateAndSort(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	values := make([]string, 0, len(set))
	for channel := range set {
		values = append(values, channel)
	}
	sort.Strings(values)
	return values
}

func sortCapabilities(items []capability_registrydto.PlatformCapabilityDTO) {
	if len(items) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Module == items[j].Module {
			return items[i].CapabilityID < items[j].CapabilityID
		}
		return items[i].Module < items[j].Module
	})
}
