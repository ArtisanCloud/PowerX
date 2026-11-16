package tenantkeys

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func legacyTestService(t *testing.T) (*TenantKeyService, *TenantKeyService) {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.Exec(`CREATE TABLE iam_tenant_key_pairs (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME, env TEXT, tenant_uuid TEXT, k_id TEXT, alg TEXT, public_pem TEXT, enc_private JSON, active BOOLEAN DEFAULT 1)`).Error; err != nil {
		t.Fatal(err)
	}
	a, err := NewDirectWrapper("a", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewDirectWrapper("b", base64.StdEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789")), nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewTenantKeyServiceWithWrapper(db, a), NewTenantKeyServiceWithWrapper(db, b)
}

func TestUnsealLegacyDuplicateKeyID(t *testing.T) {
	for _, changedMaster := range []bool{false, true} {
		t.Run(fmt.Sprintf("changed_master_%t", changedMaster), func(t *testing.T) {
			a, b := legacyTestService(t)
			if !changedMaster {
				b = a
			}
			ctx := context.Background()
			old, err := a.EnsureActiveKeyPair(ctx, "dev", "tenant-a")
			if err != nil {
				t.Fatal(err)
			}
			historical, err := a.SealSensitive(ctx, "dev", "tenant-a", datatypes.JSONMap{"api_key": "historical"}, "api_key")
			if err != nil {
				t.Fatal(err)
			}
			current, err := b.RotateKeyPair(ctx, "dev", "tenant-a")
			if err != nil {
				t.Fatal(err)
			}
			// Reproduce the legacy deployment: two distinct RSA keys share one KID.
			if err := a.db.Model(current).Update("k_id", old.KID).Error; err != nil {
				t.Fatal(err)
			}
			sealed, err := b.SealSensitive(ctx, "dev", "tenant-a", datatypes.JSONMap{"api_key": "current"}, "api_key")
			if err != nil {
				t.Fatal(err)
			}
			if !changedMaster {
				var historicalOut map[string]any
				if err := a.UnsealSensitive(ctx, "dev", "tenant-a", historical, &historicalOut); err != nil {
					t.Fatal(err)
				}
				if historicalOut["api_key"] != "historical" {
					t.Fatal("inactive historical key lost")
				}
			}
			// Make the wrong RSA key the first candidate, including when it unwraps OK.
			if err := a.db.Model(old).Update("active", true).Error; err != nil {
				t.Fatal(err)
			}
			if err := a.db.Model(current).Update("active", false).Error; err != nil {
				t.Fatal(err)
			}
			got := map[string]any{}
			if err := b.UnsealSensitive(ctx, "dev", "tenant-a", sealed, &got); err != nil {
				t.Fatal(err)
			}
			if got["api_key"] != "current" {
				t.Fatal("wrong credential")
			}
			if !changedMaster {
				if err := a.UnsealSensitive(ctx, "dev", "tenant-a", historical, &got); err != nil {
					t.Fatal(err)
				}
				if got["api_key"] != "historical" {
					t.Fatal("historical key lost")
				}
			}
			for _, scope := range [][2]string{{"prod", "tenant-a"}, {"dev", "tenant-b"}} {
				if err := b.UnsealSensitive(ctx, scope[0], scope[1], sealed, &got); err == nil {
					t.Fatal("scope isolation bypass")
				}
			}
			if err := a.db.Delete(current).Error; err != nil {
				t.Fatal(err)
			}
			got = map[string]any{"unchanged": true}
			if err := b.UnsealSensitive(ctx, "dev", "tenant-a", sealed, &got); err == nil {
				t.Fatal("expected no matching key")
			}
			if len(got) != 1 || got["unchanged"] != true {
				t.Fatal("failure mutated output")
			}
		})
	}
}

func TestKeyVersionsReserveInactiveAndDeletedHistory(t *testing.T) {
	s, _ := legacyTestService(t)
	ctx := context.Background()
	first, err := s.EnsureActiveKeyPair(ctx, "dev", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.RotateKeyPair(ctx, "dev", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if first.KID == second.KID {
		t.Fatal("rotation reused identifier")
	}
	if err := s.db.Delete(second).Error; err != nil {
		t.Fatal(err)
	}
	third, err := s.EnsureActiveKeyPair(ctx, "dev", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if third.KID == first.KID || third.KID == second.KID {
		t.Fatal("recreation reused historical identifier")
	}
	if err := s.db.Model(third).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	fourth, err := s.RotateKeyPair(ctx, "dev", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if fourth.KID == third.KID {
		t.Fatal("rotation without active key reused identifier")
	}
}

func TestConcurrentRotationsHaveDistinctKeyIDs(t *testing.T) {
	s, _ := legacyTestService(t)
	ctx := context.Background()
	const count = 4
	results := make(chan string, count)
	failures := make(chan error, count)
	for i := 0; i < count; i++ {
		go func() {
			kp, err := s.RotateKeyPair(ctx, "dev", "concurrent-tenant")
			if err != nil {
				failures <- err
				return
			}
			results <- kp.KID
		}()
	}
	seen := map[string]bool{}
	for i := 0; i < count; i++ {
		select {
		case id := <-results:
			if seen[id] {
				t.Errorf("reused key ID %s", id)
			}
			seen[id] = true
		case err := <-failures:
			t.Error(err)
		}
	}
	var active int64
	if err := s.db.Model(&modeltenant.TenantKeyPair{}).Where("env = ? AND tenant_uuid = ? AND active = ?", "dev", "concurrent-tenant", true).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active keys = %d", active)
	}
}
