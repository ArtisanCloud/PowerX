package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	capabilityregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

const TagManageCapabilityID = "com.corex.metadata.tag.manage"

// TagUpdateRequest is the fixed Core binding; it cannot choose a tenant or proxy target.
type TagUpdateRequest struct {
	Operation       string             `json:"operation"`
	TagUUID         string             `json:"tag_uuid"`
	LabelI18n       *map[string]string `json:"label_i18n"`
	DescriptionI18n *map[string]string `json:"description_i18n"`
	Color           *string            `json:"color"`
	Status          *string            `json:"status"`
}

type TagCapabilityInvoker struct{ db *gorm.DB }

func NewTagCapabilityInvoker(db *gorm.DB) *TagCapabilityInvoker {
	return &TagCapabilityInvoker{db: db}
}

func (i *TagCapabilityInvoker) InvokeCoreCapability(ctx context.Context, in capabilityregistry.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID != TagManageCapabilityID {
		return nil, capabilityregistry.ErrCoreCapabilityNotHandled
	}
	if in.Method != "INVOKE" || in.Endpoint != "core://metadata/tags" {
		return nil, invalid(errors.New("expected INVOKE core://metadata/tags"))
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return nil, invalid(err)
	}
	var request TagUpdateRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		return nil, invalid(err)
	}
	if request.Operation != "update" {
		return nil, invalid(errors.New("unsupported tag operation"))
	}
	tenant, err := NewHostContractAccess(i.db).Authorize(ctx, reqctx.AuthenticatedAPIKeyHash(ctx), "tag", "manage")
	if err != nil {
		return nil, err
	}
	if tenant != in.TenantUUID {
		return nil, forbidden(errors.New("tenant context mismatch"))
	}
	service, err := NewService(Deps{DB: i.db})
	if err != nil {
		return nil, upstream(err)
	}
	item, err := service.UpdateTag(ctx, UpdateTagInput{TenantUUID: tenant, TagUUID: request.TagUUID, LabelI18n: request.LabelI18n, DescriptionI18n: request.DescriptionI18n, Color: request.Color, Status: request.Status})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return nil, notFound(err)
		case errors.Is(err, ErrUUIDRequired), errors.Is(err, ErrMissingRequiredLocale), errors.Is(err, ErrInvalidStatus):
			return nil, invalid(err)
		default:
			return nil, upstream(err)
		}
	}
	return map[string]interface{}{"item": item}, nil
}
