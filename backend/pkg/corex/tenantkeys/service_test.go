package tenantkeys

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSealSensitiveRejectsUnreadableActiveKeyPair(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })
	if err := db.Exec(`
CREATE TABLE IF NOT EXISTS iam_tenant_key_pairs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	env TEXT,
	tenant_uuid TEXT,
	k_id TEXT,
	alg TEXT,
	public_pem TEXT,
	enc_private JSON,
	active BOOLEAN DEFAULT 1
)`).Error; err != nil {
		t.Fatalf("create tenant key table: %v", err)
	}

	keyA := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	wrapperA, err := NewDirectWrapper("test-a", keyA, nil)
	if err != nil {
		t.Fatalf("wrapper a: %v", err)
	}
	keyB := base64.StdEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789"))
	wrapperB, err := NewDirectWrapper("test-b", keyB, nil)
	if err != nil {
		t.Fatalf("wrapper b: %v", err)
	}

	ctx := context.Background()
	const env = "dev"
	const tenantUUID = "11111111-1111-1111-1111-111111111111"
	if _, err := NewTenantKeyServiceWithWrapper(db, wrapperA).EnsureActiveKeyPair(ctx, env, tenantUUID); err != nil {
		t.Fatalf("ensure key pair: %v", err)
	}

	data := datatypes.JSONMap{"api_key": "sk-test"}
	_, err = NewTenantKeyServiceWithWrapper(db, wrapperB).SealSensitive(ctx, env, tenantUUID, data, "api_key")
	if err == nil {
		t.Fatal("expected unreadable active key pair error")
	}
	if !strings.Contains(err.Error(), "active tenant key pair is not readable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
