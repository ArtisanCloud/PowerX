package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGroupMiddlewareRetainsOperationalRoutePrefix(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "routes.go")
	source := `package fixture
func routes(){
 g := protected.Group("/admin/ops/backup", RequireRoot())
 nested := g.Group("/files", guard, anotherGuard())
 nested.GET("/:id", handler)
 empty := nested.Group("", middleware())
 empty.POST("/:id/check", handler)
}`
	if e := os.WriteFile(file, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	rows, e := scanGinRoutes(dir, file)
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 2 || rows[0].Path != "/api/v1/admin/ops/backup/files/:id" || rows[1].Path != "/api/v1/admin/ops/backup/files/:id/check" {
		t.Fatalf("middleware dropped group identity: %+v", rows)
	}
}
