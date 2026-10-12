package customer

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const CustomerImportMaxRows = 200
const CustomerImportMaxBytes = 1048576
const customerImportVersion = 1

var CustomerImportColumns = []string{"type", "display_name", "primary_email", "primary_phone", "primary_contact_name", "primary_contact_email", "primary_contact_phone"}

type CustomerImportInput struct {
	Type                string `json:"type"`
	DisplayName         string `json:"display_name"`
	PrimaryEmail        string `json:"primary_email"`
	PrimaryPhone        string `json:"primary_phone"`
	PrimaryContactName  string `json:"primary_contact_name"`
	PrimaryContactEmail string `json:"primary_contact_email"`
	PrimaryContactPhone string `json:"primary_contact_phone"`
}

type CustomerImportRow struct {
	Row                int                 `json:"row"`
	Input              CustomerImportInput `json:"input"`
	Status             string              `json:"status"`
	ErrorCode          string              `json:"error_code,omitempty"`
	CustomerUUID       string              `json:"customer_uuid,omitempty"`
	PrimaryContactUUID string              `json:"primary_contact_uuid,omitempty"`
}

type CustomerImportResult struct {
	UUID      string              `json:"uuid"`
	FileName  string              `json:"file_name"`
	Checksum  string              `json:"checksum"`
	Version   int                 `json:"version"`
	Status    string              `json:"status"`
	ExpiresAt time.Time           `json:"expires_at"`
	CreatedAt time.Time           `json:"created_at"`
	TraceID   string              `json:"trace_id,omitempty"`
	Rows      []CustomerImportRow `json:"rows"`
	Total     int                 `json:"total"`
	Ready     int                 `json:"ready"`
	Created   int                 `json:"created"`
	Skipped   int                 `json:"skipped"`
	Failed    int                 `json:"failed"`
}

type AccountImportService struct {
	db   *gorm.DB
	repo *customerrepo.AccountImportRepository
}

func NewAccountImportService(db *gorm.DB) *AccountImportService {
	return &AccountImportService{db: db, repo: customerrepo.NewAccountImportRepository(db)}
}

func importError(status int, code, message string) error {
	return dto.NewErrorWithCode(status, code, message, nil)
}

func (s *AccountImportService) Preview(ctx context.Context, fileName, content string) (CustomerImportResult, error) {
	tenant, actor, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_imports_preview", "create")
	if err != nil {
		return CustomerImportResult{}, err
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" || filepath.Base(fileName) != fileName || strings.Contains(fileName, "\\") || utf8.RuneCountInString(fileName) > 160 || !strings.HasSuffix(strings.ToLower(fileName), ".csv") {
		return CustomerImportResult{}, importError(400, "CUSTOMER_IMPORT_FILE_INVALID", "请选择 UTF-8 CSV 文件")
	}
	rows, err := parseCustomerImportCSV(content)
	if err != nil {
		return CustomerImportResult{}, err
	}
	seen := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		if row.Status != "ready" {
			continue
		}
		keys := customerImportIdentityKeys(row.Input)
		duplicate := false
		for _, key := range keys {
			duplicate = duplicate || seen[key]
		}
		if duplicate {
			row.Status, row.ErrorCode = "duplicate", "CUSTOMER_IMPORT_DUPLICATE_IN_FILE"
			continue
		}
		exists, err := s.repo.IdentityExists(ctx, tenant, row.Input.PrimaryEmail, row.Input.PrimaryPhone)
		if err != nil {
			return CustomerImportResult{}, importError(503, "CUSTOMER_IMPORT_UNAVAILABLE", "客户重复校验不可用")
		}
		if exists {
			row.Status, row.ErrorCode = "duplicate", "CUSTOMER_IMPORT_DUPLICATE_EXISTING"
			continue
		}
		for _, key := range keys {
			seen[key] = true
		}
	}
	hash := sha256.Sum256([]byte(content))
	encoded, _ := json.Marshal(rows)
	batch := modelcustomer.AccountImport{TenantUUID: tenant, ActorUserID: actor, Mode: "create", FileName: fileName, Checksum: hex.EncodeToString(hash[:]), Version: customerImportVersion, Status: "preview", Rows: encoded, ExpiresAt: time.Now().UTC().Add(24 * time.Hour), TraceID: reqctx.GetTraceID(ctx)}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := customerrepo.NewAccountImportRepository(tx)
		if _, err := repo.Create(ctx, &batch); err != nil {
			return err
		}
		return recordCustomerImportAudit(ctx, repo, batch, "customer.account_import.previewed", len(rows))
	})
	if err != nil {
		return CustomerImportResult{}, importError(503, "CUSTOMER_IMPORT_UNAVAILABLE", "无法保存导入预览，请确认数据库迁移已完成")
	}
	return customerImportResult(batch)
}

func (s *AccountImportService) Get(ctx context.Context, id string) (CustomerImportResult, error) {
	tenant, actor, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_imports_import_uuid", "read")
	if err != nil {
		return CustomerImportResult{}, err
	}
	if !validImportUUID(id) {
		return CustomerImportResult{}, importError(400, "CUSTOMER_IMPORT_ID_INVALID", "导入 UUID 无效")
	}
	batch, err := s.repo.Get(ctx, tenant, actor, id, false)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CustomerImportResult{}, importError(404, "CUSTOMER_IMPORT_NOT_FOUND", "导入记录不存在")
	}
	if err != nil {
		return CustomerImportResult{}, importError(503, "CUSTOMER_IMPORT_UNAVAILABLE", "无法读取导入记录")
	}
	if batch.Mode != "create" {
		return CustomerImportResult{}, importError(404, "CUSTOMER_IMPORT_NOT_FOUND", "导入记录不存在")
	}
	return customerImportResult(batch)
}

func (s *AccountImportService) Commit(ctx context.Context, id, checksum string, confirm bool) (CustomerImportResult, error) {
	tenant, actor, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_imports_import_uuid_commit", "create")
	if err != nil {
		return CustomerImportResult{}, err
	}
	if !validImportUUID(id) || !confirm || len(checksum) != 64 {
		return CustomerImportResult{}, importError(400, "CUSTOMER_IMPORT_CONFIRM_REQUIRED", "需要确认准确的导入预览及校验和")
	}
	var result CustomerImportResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := customerrepo.NewAccountImportRepository(tx)
		if err := repo.LockTenant(ctx, tenant); err != nil {
			return err
		}
		batch, err := repo.Get(ctx, tenant, actor, id, true)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return importError(404, "CUSTOMER_IMPORT_NOT_FOUND", "导入记录不存在")
		}
		if err != nil {
			return err
		}
		if batch.Mode != "create" {
			return importError(404, "CUSTOMER_IMPORT_NOT_FOUND", "导入记录不存在")
		}
		if checksum != batch.Checksum || batch.Version != customerImportVersion {
			return importError(409, "CUSTOMER_IMPORT_PREVIEW_CHANGED", "导入预览不匹配，请重新预览")
		}
		result, err = customerImportResult(batch)
		if err != nil {
			return err
		}
		if batch.Status == "completed" {
			return nil
		}
		if time.Now().After(batch.ExpiresAt) {
			return importError(409, "CUSTOMER_IMPORT_EXPIRED", "导入预览已过期，请重新上传")
		}
		if result.Ready == 0 {
			return importError(409, "CUSTOMER_IMPORT_NO_READY_ROWS", "没有可以创建的客户行")
		}
		for i := range result.Rows {
			row := &result.Rows[i]
			if row.Status != "ready" {
				continue
			}
			exists, err := repo.IdentityExists(ctx, tenant, row.Input.PrimaryEmail, row.Input.PrimaryPhone)
			if err != nil {
				return err
			}
			if exists {
				row.Status, row.ErrorCode = "duplicate", "CUSTOMER_IMPORT_DUPLICATE_EXISTING"
				continue
			}
			created, err := NewAccountService(tx).Create(ctx, row.Input.accountInput(tenant))
			if err != nil {
				row.Status, row.ErrorCode = "failed", "CUSTOMER_IMPORT_ROW_CREATE_FAILED"
				continue
			}
			row.Status, row.CustomerUUID, row.PrimaryContactUUID = "created", created.UUID, created.PrimaryContactUUID
		}
		batch.Rows, err = json.Marshal(result.Rows)
		if err != nil {
			return err
		}
		batch.Status = "completed"
		if trace := reqctx.GetTraceID(ctx); trace != "" {
			batch.TraceID = trace
		}
		if err = repo.SaveResult(ctx, &batch); err != nil {
			return err
		}
		if err = recordCustomerImportAudit(ctx, repo, batch, "customer.account_import.committed", len(result.Rows)); err != nil {
			return err
		}
		result, err = customerImportResult(batch)
		return err
	})
	if err != nil {
		if dto.CodeOf(err) != "" {
			return CustomerImportResult{}, err
		}
		return CustomerImportResult{}, importError(503, "CUSTOMER_IMPORT_COMMIT_FAILED", "导入事务未完成，请使用同一预览重试")
	}
	return result, nil
}

func validImportUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil
}

func customerImportResult(batch modelcustomer.AccountImport) (CustomerImportResult, error) {
	result := CustomerImportResult{UUID: batch.UUID.String(), FileName: batch.FileName, Checksum: batch.Checksum, Version: batch.Version, Status: batch.Status, ExpiresAt: batch.ExpiresAt, CreatedAt: batch.CreatedAt, TraceID: batch.TraceID}
	if err := json.Unmarshal(batch.Rows, &result.Rows); err != nil {
		return result, importError(503, "CUSTOMER_IMPORT_SNAPSHOT_INVALID", "导入快照不可读")
	}
	result.Total = len(result.Rows)
	for _, row := range result.Rows {
		switch row.Status {
		case "ready":
			result.Ready++
		case "created":
			result.Created++
		case "failed":
			result.Failed++
		default:
			result.Skipped++
		}
	}
	return result, nil
}

func customerImportIdentityKeys(in CustomerImportInput) []string {
	keys := []string{}
	if in.PrimaryEmail != "" {
		keys = append(keys, "email:"+strings.ToLower(in.PrimaryEmail))
	}
	if in.PrimaryPhone != "" {
		keys = append(keys, "phone:"+customerrepo.NormalizeImportPhone(in.PrimaryPhone))
	}
	return keys
}

func (in CustomerImportInput) accountInput(tenant string) CreateAccountInput {
	out := CreateAccountInput{TenantUUID: tenant, Type: in.Type, DisplayName: in.DisplayName, PrimaryEmail: in.PrimaryEmail, PrimaryPhone: in.PrimaryPhone, Status: "active", MemberSource: "admin_csv"}
	if in.PrimaryContactName != "" || in.PrimaryContactEmail != "" || in.PrimaryContactPhone != "" {
		out.PrimaryContact = &PrimaryContactInput{DisplayName: in.PrimaryContactName, Email: in.PrimaryContactEmail, Phone: in.PrimaryContactPhone}
	}
	return out
}

func parseCustomerImportCSV(content string) ([]CustomerImportRow, error) {
	if len(content) > CustomerImportMaxBytes {
		return nil, importError(413, "CUSTOMER_IMPORT_TOO_LARGE", "CSV 文件不得超过 1 MiB 或 200 行")
	}
	if !utf8.ValidString(content) {
		return nil, importError(400, "CUSTOMER_IMPORT_ENCODING_INVALID", "CSV 必须使用 UTF-8 编码")
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(content, "\ufeff")))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, importError(400, "CUSTOMER_IMPORT_CSV_INVALID", "CSV 缺少有效表头")
	}
	allowed, seen := map[string]bool{}, map[string]bool{}
	for _, col := range CustomerImportColumns {
		allowed[col] = true
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
		if !allowed[header[i]] || seen[header[i]] {
			return nil, importError(400, "CUSTOMER_IMPORT_COLUMNS_INVALID", "CSV 含未知、重复或身份覆盖列，请使用模板")
		}
		seen[header[i]] = true
	}
	if !seen["display_name"] || (!seen["primary_email"] && !seen["primary_phone"]) {
		return nil, importError(400, "CUSTOMER_IMPORT_COLUMNS_INVALID", "CSV 必须包含 display_name 及 primary_email 或 primary_phone")
	}
	rows := []CustomerImportRow{}
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, importError(400, "CUSTOMER_IMPORT_CSV_INVALID", "CSV 格式无效，请检查引号和分隔符")
		}
		if len(rows) >= CustomerImportMaxRows {
			return nil, importError(413, "CUSTOMER_IMPORT_TOO_LARGE", "CSV 文件不得超过 1 MiB 或 200 行")
		}
		line, _ := reader.FieldPos(0)
		row := CustomerImportRow{Row: line, Status: "ready"}
		if len(record) != len(header) {
			row.Status, row.ErrorCode = "invalid", "CUSTOMER_IMPORT_ROW_COLUMNS_INVALID"
			rows = append(rows, row)
			continue
		}
		values := map[string]string{}
		for i, value := range record {
			values[header[i]] = strings.TrimSpace(value)
		}
		row.Input = CustomerImportInput{Type: values["type"], DisplayName: values["display_name"], PrimaryEmail: values["primary_email"], PrimaryPhone: values["primary_phone"], PrimaryContactName: values["primary_contact_name"], PrimaryContactEmail: values["primary_contact_email"], PrimaryContactPhone: values["primary_contact_phone"]}
		if row.Input.Type == "" {
			row.Input.Type = "person"
		}
		in := row.Input
		_, err = validateAccountPatch(AccountProfilePatch{DisplayName: &in.DisplayName, PrimaryEmail: &in.PrimaryEmail, PrimaryPhone: &in.PrimaryPhone})
		if err != nil || (in.PrimaryEmail == "" && in.PrimaryPhone == "") || (in.Type != "person" && in.Type != "company") {
			row.Status, row.ErrorCode = "invalid", "CUSTOMER_IMPORT_ROW_INVALID"
		} else if in.Type == "company" && in.PrimaryContactName == "" {
			row.Status, row.ErrorCode = "invalid", "CUSTOMER_IMPORT_PRIMARY_CONTACT_REQUIRED"
		} else if in.PrimaryContactName != "" || in.PrimaryContactEmail != "" || in.PrimaryContactPhone != "" {
			_, err = validateAccountPatch(AccountProfilePatch{DisplayName: &in.PrimaryContactName, PrimaryEmail: &in.PrimaryContactEmail, PrimaryPhone: &in.PrimaryContactPhone})
			if err != nil {
				row.Status, row.ErrorCode = "invalid", "CUSTOMER_IMPORT_ROW_INVALID"
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, importError(400, "CUSTOMER_IMPORT_EMPTY", "CSV 没有客户数据")
	}
	return rows, nil
}

func recordCustomerImportAudit(ctx context.Context, repo *customerrepo.AccountImportRepository, batch modelcustomer.AccountImport, operation string, total int) error {
	actor := int64(batch.ActorUserID)
	source, resource, idField := "customer.account_import.service", "customer_account_import", "import_uuid"
	if batch.Mode == "contact_repair" {
		source, resource, idField = "customer.contact_repair.service", "customer_contact_repair", "repair_uuid"
	}
	changes, _ := json.Marshal(map[string]any{idField: batch.UUID.String(), "checksum": batch.Checksum, "total": total, "request_id": ctx.Value("request_id")})
	return repo.RecordAudit(ctx, &modelaudit.AuditEvent{OccurredAt: time.Now().UTC(), Source: source, Outcome: "SUCCESS", Severity: "INFO", TenantUUID: batch.TenantUUID, ActorUserID: &actor, Operation: operation, ResourceType: resource, ResourceID: batch.UUID.String(), CorrelationID: reqctx.GetTraceID(ctx), ChangesAfter: datatypes.JSON(changes)})
}
