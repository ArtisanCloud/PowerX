package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCustomerAdminImportOpenAPI(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../../docs/contracts/customer-admin-import.openapi.yaml"))
	loader := openapi3.NewLoader()
	contract, err := loader.LoadFromFile(path)
	require.NoError(t, err)
	require.NoError(t, contract.Validate(context.Background()))
	for _, path := range []string{"/admin/customers/accounts", "/admin/customers/imports/preview", "/admin/customers/imports/{import_uuid}", "/admin/customers/imports/{import_uuid}/commit"} {
		require.NotNil(t, contract.Paths.Find(path))
	}
	maintenance, err := loader.LoadFromFile(filepath.Join(filepath.Dir(path), "customer-contact-maintenance.openapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, maintenance.Validate(context.Background()))
	require.Len(t, maintenance.Paths, 4)
}

func TestCustomerImportRejectsUnknownRequestFields(t *testing.T) {
	for _, body := range []string{`{"file_name":"a.csv","csv":"display_name,primary_email","tenant_uuid":"override"}`, `{"checksum":"hash","confirm":true,"rows":[]}`, `{}` + `{}`} {
		router := gin.New()
		router.POST("/test", func(c *gin.Context) {
			var request customerImportPreviewRequest
			if bindCustomerImport(c, &request) {
				c.Status(200)
			}
		})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", "/test", strings.NewReader(body)))
		require.Equal(t, 400, response.Code)
		require.Contains(t, response.Body.String(), "CUSTOMER_IMPORT_REQUEST_INVALID")
	}
}

// 本机 PostgreSQL 的独立 schema；用真实 Gin Handler/数据库验证，未替代生产登录验收。
func TestCustomerAdminImportHTTPPostgres(t *testing.T) {
	dsn := os.Getenv("POWERX_CUSTOMER_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("opt-in native PostgreSQL HTTP import acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	previousSchema := coremodel.PowerXSchema
	schema := "customer_admin_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	coremodel.PowerXSchema = schema
	t.Cleanup(func() {
		coremodel.PowerXSchema = previousSchema
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.AutoMigrate(&modelcustomer.Account{}, &modelcustomer.TenantMembership{}, &modelcustomer.Contact{}, &modelcustomer.AuthIdentity{}, &modelcustomer.AccountImport{}, &modelaudit.AuditEvent{}, &im.Permission{}, &im.Role{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}))
	tenant := uuid.NewString()
	root := reqctx.WithTraceID(reqctx.WithUserID(reqctx.WithIsRoot(reqctx.WithTenantUUID(context.Background(), tenant), true), 42), "customer-admin-http-"+uuid.NewString())
	h := &Handler{svc: customersvc.NewAccountService(db), imports: customersvc.NewAccountImportService(db), contacts: customersvc.NewContactService(db), repairs: customersvc.NewContactRepairService(db)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := root
		if c.GetHeader("X-Test-Tenant") != "" {
			ctx = reqctx.WithTenantUUID(ctx, c.GetHeader("X-Test-Tenant"))
		}
		if c.GetHeader("X-Test-Plane") != "" {
			claims := &reqctx.CoreXClaims{}
			if c.GetHeader("X-Test-Plane") == "sts" {
				claims.Issuer = "powerx-sts"
			} else {
				claims.Platforms = []string{"api_key"}
			}
			ctx = reqctx.WithClaims(ctx, claims)
		}
		c.Request = c.Request.WithContext(ctx)
	})
	router.POST("/accounts", h.CreateAccount)
	router.POST("/imports/preview", h.PreviewAccountImport)
	router.GET("/imports/:import_uuid", h.GetAccountImport)
	router.POST("/imports/:import_uuid/commit", h.CommitAccountImport)
	router.POST("/:customer_uuid/contacts", h.CreateContact)
	router.PATCH("/:customer_uuid/contacts/:contact_uuid", h.UpdateContact)
	router.POST("/:customer_uuid/contacts/:contact_uuid/primary", h.SetPrimaryContact)
	router.POST("/repairs/preview", h.PreviewContactRepair)
	router.GET("/repairs/:repair_uuid", h.GetContactRepair)
	router.POST("/repairs/:repair_uuid/commit", h.CommitContactRepair)
	call := func(method, path string, payload any, headers map[string]string) (int, []byte) {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response.Code, response.Body.Bytes()
	}
	status, body := call("POST", "/accounts", map[string]any{"type": "company", "display_name": "Manual company", "primary_contact": map[string]string{"display_name": "Jane"}}, nil)
	require.Equal(t, 201, status, string(body))
	payload := func(body []byte) map[string]any {
		var value map[string]any
		require.NoError(t, json.Unmarshal(body, &value))
		return value["data"].(map[string]any)["payload"].(map[string]any)
	}
	company := payload(body)
	status, _ = call("POST", "/accounts", map[string]any{"type": "company", "display_name": "Incomplete"}, nil)
	require.Equal(t, 400, status)
	csv := "display_name,primary_email\nAda,ada@example.test\n"
	for _, plane := range []string{"sts", "api_key"} {
		status, _ = call("POST", "/imports/preview", map[string]string{"file_name": "a.csv", "csv": csv}, map[string]string{"X-Test-Plane": plane})
		require.Equal(t, 403, status)
	}
	status, body = call("POST", "/imports/preview", map[string]string{"file_name": "a.csv", "csv": csv}, nil)
	require.Equal(t, 201, status, string(body))
	var envelope struct {
		Data struct {
			Payload customersvc.CustomerImportResult `json:"payload"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	preview := envelope.Data.Payload
	require.Equal(t, 1, preview.Ready)
	status, _ = call("GET", "/imports/"+preview.UUID, nil, map[string]string{"X-Test-Tenant": uuid.NewString()})
	require.Equal(t, 404, status)
	status, _ = call("POST", "/imports/"+preview.UUID+"/commit", map[string]any{"checksum": strings.Repeat("b", 64), "confirm": true}, nil)
	require.Equal(t, 409, status)
	// 两个同时到达的确认只创建一位客户；不同请求读取同一持久化结果。
	var wg sync.WaitGroup
	statuses := [2]int{}
	bodies := [2][]byte{}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], bodies[i] = call("POST", "/imports/"+preview.UUID+"/commit", map[string]any{"checksum": preview.Checksum, "confirm": true}, nil)
		}(i)
	}
	wg.Wait()
	for i := range statuses {
		require.Equal(t, 200, statuses[i], string(bodies[i]))
		require.NoError(t, json.Unmarshal(bodies[i], &envelope))
		require.Equal(t, 1, envelope.Data.Payload.Created)
		require.NotEmpty(t, envelope.Data.Payload.Rows[0].PrimaryContactUUID)
	}
	var count int64
	require.NoError(t, db.Model(&modelcustomer.Account{}).Where("primary_email = ?", "ada@example.test").Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&modelcustomer.AuthIdentity{}).Count(&count).Error)
	require.Zero(t, count, "administrative creation must not create login credentials")
	status, body = call("GET", "/imports/"+preview.UUID, nil, nil)
	require.Equal(t, 200, status)
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Equal(t, "completed", envelope.Data.Payload.Status)
	companyID := company["uuid"].(string)
	oldPrimary := company["primary_contact_uuid"].(string)
	status, body = call("PATCH", "/"+companyID+"/contacts/"+oldPrimary, map[string]string{"status": "inactive"}, nil)
	require.Equal(t, 409, status, string(body))
	require.Contains(t, string(body), "CONTACT_PRIMARY_CHANGE_REQUIRED")
	status, body = call("POST", "/"+companyID+"/contacts", map[string]any{"display_name": strings.Repeat("中", 43), "creation_intent": "explicit_create"}, nil)
	require.Equal(t, 201, status, string(body))
	target := payload(body)["uuid"].(string)
	status, body = call("POST", "/"+companyID+"/contacts/"+target+"/primary", map[string]string{"expected_primary_contact_uuid": oldPrimary}, map[string]string{"X-Test-Tenant": uuid.NewString()})
	require.NotEqual(t, 200, status, string(body))
	status, body = call("POST", "/"+companyID+"/contacts/"+target+"/primary", map[string]string{"expected_primary_contact_uuid": oldPrimary}, map[string]string{"X-Test-Plane": "api_key"})
	require.Equal(t, 403, status, string(body))
	// PostgreSQL 锁竞争：设主与停用并发，最终引用只能指向 active 联系人。
	var raceCodes [2]int
	var raceBodies [2][]byte
	var primaryRace sync.WaitGroup
	primaryRace.Add(2)
	go func() {
		defer primaryRace.Done()
		raceCodes[0], raceBodies[0] = call("POST", "/"+companyID+"/contacts/"+target+"/primary", map[string]string{"expected_primary_contact_uuid": oldPrimary}, nil)
	}()
	go func() {
		defer primaryRace.Done()
		raceCodes[1], raceBodies[1] = call("PATCH", "/"+companyID+"/contacts/"+target, map[string]string{"status": "inactive"}, nil)
	}()
	primaryRace.Wait()
	require.Contains(t, []int{200, 409}, raceCodes[0], string(raceBodies[0]))
	require.Contains(t, []int{200, 409}, raceCodes[1], string(raceBodies[1]))
	var membership modelcustomer.TenantMembership
	require.NoError(t, db.Where("tenant_uuid = ? AND customer_uuid = ?", tenant, companyID).First(&membership).Error)
	var activePrimary modelcustomer.Contact
	require.NoError(t, db.Where("uuid = ? AND status = ?", membership.PrimaryContactUUID, "active").First(&activePrimary).Error)
	legacy := modelcustomer.Account{DisplayName: "Legacy", Status: "active"}
	require.NoError(t, db.Create(&legacy).Error)
	legacyMember := modelcustomer.TenantMembership{TenantUUID: tenant, CustomerUUID: legacy.UUID.String(), Status: "active", Source: "platform"}
	require.NoError(t, db.Omit("primary_contact_uuid").Create(&legacyMember).Error)
	status, body = call("POST", "/repairs/preview", map[string]string{"customer_uuid": legacy.UUID.String(), "type": "person"}, nil)
	require.Equal(t, 201, status, string(body))
	repair := payload(body)
	status, body = call("GET", "/repairs/"+repair["uuid"].(string), nil, map[string]string{"X-Test-Tenant": uuid.NewString()})
	require.Equal(t, 404, status, string(body))
	for i := 0; i < 2; i++ {
		status, body = call("POST", "/repairs/"+repair["uuid"].(string)+"/commit", map[string]any{"checksum": repair["checksum"], "confirm": true}, nil)
		require.Equal(t, 200, status, string(body))
		require.Equal(t, "completed", payload(body)["status"])
	}
	var repaired int64
	require.NoError(t, db.Model(&modelcustomer.Contact{}).Where("tenant_uuid = ? AND customer_uuid = ?", tenant, legacy.UUID.String()).Count(&repaired).Error)
	require.EqualValues(t, 1, repaired)
	t.Logf("native HTTP schema=%s import=%s repair=%s trace=%s; primary/status locking and idempotent repair passed; schema cleaned at teardown", schema, preview.UUID, repair["uuid"], envelope.Data.Payload.TraceID)
}
