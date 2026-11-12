package notifications

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const CreateReceiptContract = "notification_create.v1"

// IdempotentCreateInput 是固定成员创建契约，不接受调用方指定租户或服务身份。
type IdempotentCreateInput struct {
	IdempotencyKey string          `json:"idempotency_key"`
	Title          string          `json:"title"`
	Content        string          `json:"content"`
	Type           string          `json:"type,omitempty"`
	Category       string          `json:"category,omitempty"`
	IsImportant    bool            `json:"is_important,omitempty"`
	MemberUUID     string          `json:"member_uuid"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	Navigation     *Navigation     `json:"navigation,omitempty"`
}

type CreateReceipt struct {
	Contract       string         `json:"contract"`
	IdempotencyKey string         `json:"idempotency_key"`
	Replayed       bool           `json:"replayed"`
	Notification   map[string]any `json:"notification"`
}

func notificationHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func canonicalUUID(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}

func (in *IdempotentCreateInput) validate() error {
	bad := func() error { return receiptError(400, "NOTIFICATION_CREATE_INVALID") }
	if len(in.IdempotencyKey) < 1 || len(in.IdempotencyKey) > 256 || !canonicalUUID(in.MemberUUID) || in.Title == "" || in.Content == "" || strings.TrimSpace(in.Title) != in.Title || strings.TrimSpace(in.Content) != in.Content || utf8.RuneCountInString(in.Title) > 255 || utf8.RuneCountInString(in.Content) > 32768 || len(in.Type) > 32 || len(in.Category) > 64 || strings.TrimSpace(in.Type) != in.Type || strings.TrimSpace(in.Category) != in.Category {
		return bad()
	}
	for _, r := range in.IdempotencyKey {
		if r < 33 || r > 126 {
			return bad()
		}
	}
	if in.Type == "" {
		in.Type = "info"
	}
	if in.Category == "" {
		in.Category = "system"
	}
	if _, err := NavigationActions(in.Navigation); err != nil {
		return err
	}
	if len(in.Metadata) > 65536 {
		return bad()
	}
	if len(in.Metadata) > 0 {
		var metadata map[string]any
		decoder := json.NewDecoder(bytes.NewReader(in.Metadata))
		decoder.UseNumber()
		if decoder.Decode(&metadata) != nil || decoder.Decode(&struct{}{}) != io.EOF {
			return bad()
		}
		var err error
		if metadata == nil {
			in.Metadata = nil
		} else {
			in.Metadata, err = json.Marshal(metadata)
		}
		if err != nil {
			return bad()
		}
	}
	return nil
}

// CreateIdempotent 在同一事务中保留去重键、通知与原始返回值。
// 重试每次重新核实授权和接收人；只返回原通知，不再次发布 created 事件。
func (s *Service) CreateIdempotent(ctx context.Context, input IdempotentCreateInput) (*CreateReceipt, error) {
	owner, err := s.authorizedOwner(ctx, CreateCapabilityID)
	if err != nil {
		return nil, err
	}
	tenant := reqctx.GetTenantUUID(ctx)
	if !canonicalUUID(tenant) {
		return nil, receiptError(400, "NOTIFICATION_CREATE_INVALID")
	}
	if err = input.validate(); err != nil {
		return nil, err
	}
	if err = NewRecipientAccess(s.db).Require(ctx, tenant, input.MemberUUID); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, receiptError(400, "NOTIFICATION_CREATE_INVALID")
	}
	keyHash, requestHash := notificationHash([]byte(input.IdempotencyKey)), notificationHash(payload)
	var receipt *CreateReceipt
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := nm.CreationReceipt{UUID: uuid.New(), TenantUUID: tenant, ServiceOwner: owner, KeyHash: keyHash, RequestHash: requestHash, NotificationUUID: uuid.New(), ResponseJSON: datatypes.JSON(`{}`)}
		insert := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_uuid"}, {Name: "service_owner"}, {Name: "key_hash"}}, DoNothing: true}).Create(&row)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			row = nm.CreationReceipt{}
			if err := tx.Where("tenant_uuid = ? AND service_owner = ? AND key_hash = ?", tenant, owner, keyHash).Take(&row).Error; err != nil {
				return err
			}
			if row.RequestHash != requestHash {
				return receiptError(409, "NOTIFICATION_IDEMPOTENCY_CONFLICT")
			}
			decoder := json.NewDecoder(bytes.NewReader(row.ResponseJSON))
			decoder.UseNumber()
			if err := decoder.Decode(&receipt); err != nil || receipt == nil || receipt.Contract != CreateReceiptContract || receipt.IdempotencyKey != input.IdempotencyKey || receipt.Notification["id"] != row.NotificationUUID.String() || receipt.Notification["userId"] != input.MemberUUID {
				return receiptError(503, "NOTIFICATION_CREATE_RECEIPT_INVALID")
			}
			receipt.Replayed = true
			return nil
		}
		scoped := *s
		scoped.db = tx
		scoped.repo = repo.NewNotificationRepository(tx)
		metadata := datatypes.JSON(input.Metadata)
		if len(metadata) == 0 {
			metadata = datatypes.JSON(`null`)
		}
		item, err := scoped.Create(ctx, CreateInput{storageUUID: row.NotificationUUID, TenantUUID: tenant, MemberUUID: input.MemberUUID, Title: input.Title, Content: input.Content, Type: input.Type, Category: input.Category, IsImportant: input.IsImportant, Metadata: metadata, Navigation: input.Navigation})
		if err != nil {
			return err
		}
		notification := CreatedEvent(item)
		// 保留 metadata 数字精度，避免通用 map 的 float64 转换改变结果。
		if len(item.Metadata) > 0 {
			notification["metadata"] = json.RawMessage(item.Metadata)
		}
		receipt = &CreateReceipt{Contract: CreateReceiptContract, IdempotencyKey: input.IdempotencyKey, Notification: notification}
		raw, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		return tx.Model(&row).Update("response_json", datatypes.JSON(raw)).Error
	})
	if err != nil {
		return nil, err
	}
	return receipt, nil
}
