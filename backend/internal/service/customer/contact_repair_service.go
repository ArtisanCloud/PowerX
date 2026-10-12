package customer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ContactRepairInput struct {
	CustomerUUID   string               `json:"customer_uuid"`
	Type           string               `json:"type"`
	ContactUUID    string               `json:"contact_uuid,omitempty"`
	PrimaryContact *PrimaryContactInput `json:"primary_contact,omitempty"`
}
type ContactRepairProposal struct {
	CustomerUUID               string               `json:"customer_uuid"`
	Type                       string               `json:"type"`
	Action                     string               `json:"action"`
	ExpectedPrimaryContactUUID string               `json:"expected_primary_contact_uuid"`
	ContactUUID                string               `json:"contact_uuid,omitempty"`
	PrimaryContact             *PrimaryContactInput `json:"primary_contact,omitempty"`
}
type contactRepairSnapshot struct {
	Input              ContactRepairInput    `json:"input"`
	Fingerprint        string                `json:"fingerprint"`
	Proposal           ContactRepairProposal `json:"proposal"`
	PrimaryContactUUID string                `json:"primary_contact_uuid,omitempty"`
}
type ContactRepairResult struct {
	UUID               string                `json:"uuid"`
	Checksum           string                `json:"checksum"`
	Status             string                `json:"status"`
	ExpiresAt          time.Time             `json:"expires_at"`
	TraceID            string                `json:"trace_id,omitempty"`
	Proposal           ContactRepairProposal `json:"proposal"`
	PrimaryContactUUID string                `json:"primary_contact_uuid,omitempty"`
}
type ContactRepairService struct{ db *gorm.DB }

func NewContactRepairService(db *gorm.DB) *ContactRepairService { return &ContactRepairService{db: db} }

func repairResult(batch modelcustomer.AccountImport) (ContactRepairResult, error) {
	var snapshot contactRepairSnapshot
	if err := json.Unmarshal(batch.Rows, &snapshot); err != nil {
		return ContactRepairResult{}, importError(503, "CONTACT_REPAIR_SNAPSHOT_INVALID", "补录快照不可读")
	}
	return ContactRepairResult{UUID: batch.UUID.String(), Checksum: batch.Checksum, Status: batch.Status, ExpiresAt: batch.ExpiresAt, TraceID: batch.TraceID, Proposal: snapshot.Proposal, PrimaryContactUUID: snapshot.PrimaryContactUUID}, nil
}

// planContactRepair 只读检查；类型、归属或数据不明确时要求显式选择，不在读取中补写。
func planContactRepair(ctx context.Context, db *gorm.DB, tenant string, input ContactRepairInput) (contactRepairSnapshot, error) {
	customerUUID, err := canonicalUUID(input.CustomerUUID)
	if err != nil {
		return contactRepairSnapshot{}, err
	}
	input.CustomerUUID = customerUUID
	if input.ContactUUID != "" {
		input.ContactUUID, err = canonicalUUID(input.ContactUUID)
		if err != nil {
			return contactRepairSnapshot{}, err
		}
	}
	if input.Type != "person" && input.Type != "company" {
		return contactRepairSnapshot{}, importError(400, "CONTACT_REPAIR_TYPE_REQUIRED", "请明确确认个人或企业类型")
	}
	var account modelcustomer.Account
	if err := db.WithContext(ctx).Where("uuid = ?", input.CustomerUUID).First(&account).Error; err != nil {
		return contactRepairSnapshot{}, importError(404, "CONTACT_REPAIR_CUSTOMER_NOT_FOUND", "客户不存在")
	}
	var membership modelcustomer.TenantMembership
	if err := db.WithContext(ctx).Where("tenant_uuid = ? AND customer_uuid = ?", tenant, input.CustomerUUID).First(&membership).Error; err != nil {
		return contactRepairSnapshot{}, importError(404, "CONTACT_REPAIR_CUSTOMER_NOT_FOUND", "客户不属于当前租户")
	}
	if account.Status != "active" || membership.Status != "active" || (membership.ExpiresAt != nil && !membership.ExpiresAt.After(time.Now())) {
		return contactRepairSnapshot{}, importError(409, "CONTACT_REPAIR_CUSTOMER_INACTIVE", "补录要求有效客户和成员关系")
	}
	if account.Type != "" && account.Type != input.Type {
		return contactRepairSnapshot{}, importError(409, "CONTACT_REPAIR_TYPE_CHANGED", "不能通过补录更改已经确认的客户类型")
	}
	contacts, err := customerrepo.NewContactRepository(db).AllForCustomer(ctx, tenant, input.CustomerUUID)
	if err != nil {
		return contactRepairSnapshot{}, err
	}
	state, _ := json.Marshal(struct {
		Account    modelcustomer.Account
		Membership modelcustomer.TenantMembership
		Contacts   []modelcustomer.Contact
	}{account, membership, contacts})
	hash := sha256.Sum256(state)
	snapshot := contactRepairSnapshot{Input: input, Fingerprint: hex.EncodeToString(hash[:]), Proposal: ContactRepairProposal{CustomerUUID: input.CustomerUUID, Type: input.Type, ExpectedPrimaryContactUUID: membership.PrimaryContactUUID}}
	active := []modelcustomer.Contact{}
	for _, contact := range contacts {
		if contact.Status == "active" {
			active = append(active, contact)
			if membership.PrimaryContactUUID == contact.UUID.String() {
				if input.PrimaryContact != nil || (input.ContactUUID != "" && input.ContactUUID != contact.UUID.String()) {
					return snapshot, importError(409, "CONTACT_REPAIR_NOT_REQUIRED", "主联系人已有效，请使用正式设主操作更换")
				}
				snapshot.Proposal.Action = "not_needed"
				if account.Type == "" {
					snapshot.Proposal.Action = "type_only"
				}
				snapshot.Proposal.ContactUUID = contact.UUID.String()
				return snapshot, nil
			}
		}
	}
	if input.ContactUUID != "" {
		if input.PrimaryContact != nil {
			return snapshot, importError(400, "CONTACT_REPAIR_INPUT_INVALID", "已有联系人与新建资料只能选择一种")
		}
		for _, contact := range active {
			if contact.UUID.String() == input.ContactUUID {
				snapshot.Proposal.Action = "reuse_contact"
				snapshot.Proposal.ContactUUID = input.ContactUUID
				return snapshot, nil
			}
		}
		return snapshot, importError(409, "CONTACT_REPAIR_CONTACT_INVALID", "目标联系人必须有效且属于此客户和当前租户")
	}
	if input.PrimaryContact == nil {
		if len(active) > 1 {
			return snapshot, importError(409, "CONTACT_REPAIR_AMBIGUOUS", "存在多位联系人，请明确选择")
		}
		if len(active) == 1 {
			snapshot.Proposal.Action = "reuse_contact"
			snapshot.Proposal.ContactUUID = active[0].UUID.String()
			return snapshot, nil
		}
		if input.Type == "company" {
			return snapshot, importError(400, "CONTACT_REPAIR_PERSON_REQUIRED", "企业补录必须明确提供自然人主要联系人")
		}
		input.PrimaryContact = &PrimaryContactInput{DisplayName: account.DisplayName, GivenName: account.GivenName, FamilyName: account.FamilyName, Email: account.PrimaryEmail, Phone: account.PrimaryPhone}
	}
	primary := *input.PrimaryContact
	primary.DisplayName = strings.TrimSpace(primary.DisplayName)
	primary.GivenName = strings.TrimSpace(primary.GivenName)
	primary.FamilyName = strings.TrimSpace(primary.FamilyName)
	primary.Email = strings.TrimSpace(primary.Email)
	primary.Phone = strings.TrimSpace(primary.Phone)
	if !validContactNames(primary.DisplayName, primary.GivenName, primary.FamilyName) || !validContactChannels(primary.Email, primary.Phone) {
		return snapshot, importError(400, "CONTACT_REPAIR_PERSON_INVALID", "自然人资料无效，请补充姓名、邮箱或电话")
	}
	snapshot.Proposal.Action = "create_contact"
	snapshot.Proposal.PrimaryContact = &primary
	return snapshot, nil
}

func (s *ContactRepairService) Preview(ctx context.Context, input ContactRepairInput) (ContactRepairResult, error) {
	tenant, actor, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_contact_repairs_preview", "create")
	if err != nil {
		return ContactRepairResult{}, err
	}
	snapshot, err := planContactRepair(ctx, s.db, tenant, input)
	if err != nil {
		return ContactRepairResult{}, err
	}
	data, _ := json.Marshal(snapshot)
	hash := sha256.Sum256(data)
	batch := modelcustomer.AccountImport{TenantUUID: tenant, ActorUserID: actor, FileName: "contact-repair", Mode: "contact_repair", Checksum: hex.EncodeToString(hash[:]), Version: 1, Status: "preview", Rows: data, ExpiresAt: time.Now().UTC().Add(24 * time.Hour), TraceID: reqctx.GetTraceID(ctx)}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := customerrepo.NewAccountImportRepository(tx)
		if _, err := repo.Create(ctx, &batch); err != nil {
			return err
		}
		return recordCustomerImportAudit(ctx, repo, batch, "customer.contact_repair.previewed", 1)
	})
	if err != nil {
		return ContactRepairResult{}, importError(503, "CONTACT_REPAIR_UNAVAILABLE", "无法保存补录预览，请确认迁移已完成")
	}
	return repairResult(batch)
}
func (s *ContactRepairService) Get(ctx context.Context, id string) (ContactRepairResult, error) {
	tenant, actor, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_contact_repairs_repair_uuid", "read")
	if err != nil {
		return ContactRepairResult{}, err
	}
	if !validImportUUID(id) {
		return ContactRepairResult{}, importError(400, "CONTACT_REPAIR_ID_INVALID", "补录 UUID 无效")
	}
	batch, err := customerrepo.NewAccountImportRepository(s.db).Get(ctx, tenant, actor, id, false)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && batch.Mode != "contact_repair") {
		return ContactRepairResult{}, importError(404, "CONTACT_REPAIR_NOT_FOUND", "补录记录不存在")
	}
	if err != nil {
		return ContactRepairResult{}, importError(503, "CONTACT_REPAIR_UNAVAILABLE", "无法读取补录记录")
	}
	return repairResult(batch)
}
func (s *ContactRepairService) Commit(ctx context.Context, id, checksum string, confirm bool) (ContactRepairResult, error) {
	tenant, actor, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_contact_repairs_repair_uuid_commit", "create")
	if err != nil {
		return ContactRepairResult{}, err
	}
	if !validImportUUID(id) || !confirm {
		return ContactRepairResult{}, importError(400, "CONTACT_REPAIR_CONFIRM_REQUIRED", "需要明确确认保存的补录预览")
	}
	var result ContactRepairResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := customerrepo.NewAccountImportRepository(tx)
		batch, err := repo.Get(ctx, tenant, actor, id, true)
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && batch.Mode != "contact_repair") {
			return importError(404, "CONTACT_REPAIR_NOT_FOUND", "补录记录不存在")
		}
		if err != nil {
			return err
		}
		if checksum != batch.Checksum || batch.Version != 1 {
			return importError(409, "CONTACT_REPAIR_PREVIEW_CHANGED", "补录预览不匹配")
		}
		if batch.Status == "completed" {
			result, err = repairResult(batch)
			return err
		}
		if !batch.ExpiresAt.After(time.Now()) {
			return importError(409, "CONTACT_REPAIR_EXPIRED", "补录预览已过期")
		}
		var saved contactRepairSnapshot
		if err = json.Unmarshal(batch.Rows, &saved); err != nil {
			return err
		}
		if _, err = customerrepo.NewContactRepository(tx).LockMembership(ctx, tenant, saved.Input.CustomerUUID); err != nil {
			return err
		}
		var account modelcustomer.Account
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ?", saved.Input.CustomerUUID).First(&account).Error; err != nil {
			return err
		}
		current, err := planContactRepair(ctx, tx, tenant, saved.Input)
		if err != nil {
			return err
		}
		if current.Fingerprint != saved.Fingerprint {
			return importError(409, "CONTACT_REPAIR_SOURCE_CHANGED", "客户或联系人已变化，请重新预览")
		}
		if account.Type == "" {
			if err = tx.Model(&account).Update("type", saved.Proposal.Type).Error; err != nil {
				return err
			}
		}
		primary := saved.Proposal.ContactUUID
		if saved.Proposal.Action == "create_contact" {
			p := saved.Proposal.PrimaryContact
			contact, err := NewContactService(tx).Create(ctx, CreateContactInput{TenantUUID: tenant, CustomerUUID: saved.Input.CustomerUUID, DisplayName: p.DisplayName, GivenName: p.GivenName, FamilyName: p.FamilyName, Email: p.Email, Phone: p.Phone, Status: "active", CreationIntent: ContactCreationIntentExplicitCreate})
			if err != nil {
				return err
			}
			primary = contact.UUID.String()
		}
		if primary != "" {
			if _, err = NewContactService(tx).SetPrimaryContact(ctx, SetPrimaryContactInput{TenantUUID: tenant, CustomerUUID: saved.Input.CustomerUUID, ContactUUID: primary, ExpectedPrimaryContactUUID: saved.Proposal.ExpectedPrimaryContactUUID}); err != nil {
				return err
			}
		}
		saved.PrimaryContactUUID = primary
		batch.Rows, _ = json.Marshal(saved)
		batch.Status = "completed"
		batch.TraceID = reqctx.GetTraceID(ctx)
		if err = repo.SaveResult(ctx, &batch); err != nil {
			return err
		}
		if err = recordCustomerImportAudit(ctx, repo, batch, "customer.contact_repair.committed", 1); err != nil {
			return err
		}
		result, err = repairResult(batch)
		return err
	})
	if err != nil {
		if code := dto.CodeOf(err); code != "" {
			return ContactRepairResult{}, err
		}
		return ContactRepairResult{}, importError(503, "CONTACT_REPAIR_COMMIT_FAILED", "补录事务未完成，可使用原预览重试")
	}
	return result, nil
}
