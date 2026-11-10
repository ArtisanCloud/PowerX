package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
)

const BusinessCardContract = "business_notification_card.v1"
const BusinessCardContractV2 = "business_notification_card.v2"
const InvalidateCardCapabilityID = "com.corex.notifications.invalidate_card"
const maxCardRevision int64 = 9007199254740991

type BusinessCardReference struct {
	Contract       string `json:"contract"`
	SourcePlugin   string `json:"source_plugin"`
	BusinessType   string `json:"business_type"`
	DeliveryKey    string `json:"delivery_key"`
	SourceRevision int64  `json:"source_revision"`
}

type PresentationSource struct {
	Contract           string `json:"contract"`
	SourcePlugin       string `json:"source_plugin"`
	PresentationURL    string `json:"presentation_url"`
	PreviewURLTemplate string `json:"preview_url_template"`
}

func cardReference(raw []byte) (*BusinessCardReference, error) {
	var metadata map[string]json.RawMessage
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if json.Unmarshal(raw, &metadata) != nil {
		return nil, receiptError(400, "NOTIFICATION_CARD_INVALID")
	}
	value, ok := metadata["business_card"]
	if !ok {
		return nil, nil
	}
	var ref BusinessCardReference
	d := json.NewDecoder(bytes.NewReader(value))
	d.DisallowUnknownFields()
	if d.Decode(&ref) != nil || d.Decode(&struct{}{}) != io.EOF || !plugin_mgr.IsBusinessCardContract(ref.Contract) || !plugin_mgr.PresentationToken.MatchString(ref.BusinessType) || !plugin_mgr.PresentationToken.MatchString(ref.DeliveryKey) || ref.SourceRevision < 1 || ref.SourceRevision > maxCardRevision {
		return nil, receiptError(400, "NOTIFICATION_CARD_INVALID")
	}
	// Plugin IDs may contain dots; URLs, slashes and traversal are never accepted.
	if ref.SourcePlugin == "" || len(ref.SourcePlugin) > 128 {
		return nil, receiptError(400, "NOTIFICATION_CARD_INVALID")
	}
	for _, part := range strings.Split(ref.SourcePlugin, ".") {
		if !plugin_mgr.PresentationToken.MatchString(part) {
			return nil, receiptError(400, "NOTIFICATION_CARD_INVALID")
		}
	}
	return &ref, nil
}

func (s *Service) presentationBinding(ctx context.Context, ref *BusinessCardReference) (plugin_mgr.NotificationPresentationSpec, string, error) {
	fail := func() (plugin_mgr.NotificationPresentationSpec, string, error) {
		return plugin_mgr.NotificationPresentationSpec{}, "", receiptError(503, "NOTIFICATION_PRESENTATION_UNAVAILABLE")
	}
	if s.LookupPresentationPlugin == nil {
		return fail()
	}
	p, err := s.LookupPresentationPlugin(ctx, ref.SourcePlugin)
	if err != nil || p.ID != ref.SourcePlugin || p.State != plugin_mgr.StateEnabled {
		return fail()
	}
	base := strings.TrimSuffix(p.Endpoints.HTTPBasePath, "/")
	if !strings.HasPrefix(base, "/api/") || strings.ContainsAny(base, "?#%\\") || strings.Contains(base, "..") || strings.Contains(base, "//") {
		return fail()
	}
	for _, binding := range p.NotificationPresentations {
		if binding.BusinessType == ref.BusinessType && binding.Contract == ref.Contract {
			if binding.Validate() != nil {
				return fail()
			}
			return binding, "/_p/" + p.ID + base, nil
		}
	}
	return fail()
}

func (s *Service) validateBusinessCard(ctx context.Context, item *nm.Notification) error {
	ref, err := cardReference(item.Metadata)
	if err != nil || ref == nil {
		return err
	}
	if item.MemberUUID == "" || item.ServiceOwner == "" {
		return receiptError(400, "NOTIFICATION_CARD_MEMBER_REQUIRED")
	}
	claims := reqctx.GetClaims(ctx)
	if claims != nil && claims.Issuer == "powerx-sts" && ref.SourcePlugin != claims.PluginID {
		return receiptError(403, "NOTIFICATION_CARD_SOURCE_FORBIDDEN")
	}
	if _, _, err = s.presentationBinding(ctx, ref); err != nil {
		return err
	}
	item.SourceRevision = ref.SourceRevision
	return nil
}

func (s *Service) PresentationSource(ctx context.Context, tenant, member, id string) (*PresentationSource, error) {
	item, err := s.Get(ctx, tenant, member, id)
	if err != nil {
		return nil, err
	}
	if item == nil || member == "" || item.MemberUUID != member {
		return nil, receiptError(404, "NOTIFICATION_NOT_FOUND")
	}
	ref, err := cardReference(item.Metadata)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, receiptError(404, "NOTIFICATION_CARD_NOT_FOUND")
	}
	binding, base, err := s.presentationBinding(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &PresentationSource{Contract: ref.Contract, SourcePlugin: ref.SourcePlugin, PresentationURL: base + strings.ReplaceAll(binding.PresentationPath, "{delivery_key}", ref.DeliveryKey), PreviewURLTemplate: base + strings.ReplaceAll(binding.PreviewPath, "{delivery_key}", ref.DeliveryKey)}, nil
}

type InvalidateCardInput struct {
	CardReference    *BusinessCardReference `json:"card_reference,omitempty"`
	Reason           string                 `json:"reason,omitempty"`
	MemberUUID       string                 `json:"member_uuid"`
	NotificationUUID string                 `json:"notification_uuid"`
	SourceRevision   int64                  `json:"source_revision"`
}

func (s *Service) InvalidateCard(ctx context.Context, input InvalidateCardInput) (*nm.Notification, bool, error) {
	owner, err := s.authorizedOwner(ctx, InvalidateCardCapabilityID)
	if err != nil {
		return nil, false, err
	}
	for _, raw := range []string{input.MemberUUID, input.NotificationUUID} {
		id, e := uuid.Parse(raw)
		if e != nil || id == uuid.Nil || id.String() != raw {
			return nil, false, receiptError(400, "NOTIFICATION_CARD_INVALID")
		}
	}
	if (input.Reason != "" && input.Reason != "content_changed" && input.Reason != "access_changed" && input.Reason != "presentation_changed") || input.SourceRevision < 1 || input.SourceRevision > maxCardRevision {
		return nil, false, receiptError(400, "NOTIFICATION_CARD_INVALID")
	}
	tenant := reqctx.GetTenantUUID(ctx)
	if err = NewRecipientAccess(s.db).Require(ctx, tenant, input.MemberUUID); err != nil {
		return nil, false, err
	}
	var attachMetadata []byte
	var attachExpected int64
	if input.CardReference != nil {
		owned, e := s.repo.FindOwned(ctx, tenant, owner, input.MemberUUID, input.NotificationUUID)
		if e != nil {
			return nil, false, receiptError(403, "NOTIFICATION_CARD_FORBIDDEN")
		}
		ref := input.CardReference
		if ref.SourceRevision != input.SourceRevision {
			return nil, false, receiptError(400, "NOTIFICATION_CARD_INVALID")
		}
		current, e := cardReference(owned.Metadata)
		if e != nil {
			return nil, false, e
		}
		if current != nil && (current.SourcePlugin != ref.SourcePlugin || current.BusinessType != ref.BusinessType || current.DeliveryKey != ref.DeliveryKey) {
			return nil, false, receiptError(409, "NOTIFICATION_CARD_BINDING_CONFLICT")
		}
		upgrade := current != nil && current.Contract == BusinessCardContract && ref.Contract == BusinessCardContractV2
		if current != nil && current.Contract != ref.Contract && !upgrade {
			return nil, false, receiptError(409, "NOTIFICATION_CARD_BINDING_CONFLICT")
		}
		if current == nil || upgrade {
			if upgrade && (input.SourceRevision <= owned.SourceRevision || input.Reason != "presentation_changed") {
				return nil, false, receiptError(409, "NOTIFICATION_CARD_BINDING_CONFLICT")
			}
			var metadata map[string]any
			if len(owned.Metadata) > 0 && json.Unmarshal(owned.Metadata, &metadata) != nil {
				return nil, false, receiptError(400, "NOTIFICATION_CARD_INVALID")
			}
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["business_card"] = ref
			raw, e := json.Marshal(metadata)
			if e != nil {
				return nil, false, e
			}
			previousSource := owned.SourceRevision
			owned.Metadata = raw
			if e = s.validateBusinessCard(ctx, owned); e != nil {
				return nil, false, e
			}
			attachMetadata = raw
			if upgrade {
				attachExpected = previousSource
			}
		}
	}
	item, changed, err := s.repo.InvalidateCard(ctx, tenant, owner, input.MemberUUID, input.NotificationUUID, input.SourceRevision, s.now(), input.Reason != "access_changed" && input.Reason != "presentation_changed", attachMetadata, attachExpected)
	if errors.Is(err, repo.ErrRevisionConflict) {
		return nil, false, receiptError(409, "NOTIFICATION_REVISION_CONFLICT")
	}
	if errors.Is(err, repo.ErrCardNotOwned) {
		return nil, false, receiptError(403, "NOTIFICATION_CARD_FORBIDDEN")
	}
	return item, changed, err
}

func (i *CapabilityInvoker) invokeInvalidateCard(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	bad := func() (map[string]interface{}, error) { return nil, receiptError(400, "NOTIFICATION_CARD_INVALID") }
	if in.Method != "INVOKE" || in.Endpoint != CapabilityEndpoint || len(in.Query) > 0 || len(in.Context) > 0 || in.TenantUUID != reqctx.GetTenantUUID(ctx) {
		return bad()
	}
	for k := range in.Payload {
		if k != "body" {
			return bad()
		}
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return bad()
	}
	var op struct {
		Operation string `json:"operation"`
		InvalidateCardInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&op) != nil || d.Decode(&struct{}{}) != io.EOF || op.Operation != "invalidate_card" {
		return bad()
	}
	if i == nil || i.Service == nil || i.Publish == nil {
		return nil, receiptError(503, "NOTIFICATION_UNAVAILABLE")
	}
	item, changed, err := i.Service.InvalidateCard(ctx, op.InvalidateCardInput)
	if err != nil {
		return nil, err
	}
	if changed {
		event := CreatedEvent(item)
		event["eventType"] = "notification_updated"
		event["payload_version"] = 1
		event["tenant_uuid"] = in.TenantUUID
		i.Publish(ctx, in.TenantUUID, event)
	}
	return map[string]interface{}{"result": map[string]any{"notification_uuid": item.UUID.String(), "revision": item.Revision, "source_revision": item.SourceRevision, "changed": changed}}, nil
}
