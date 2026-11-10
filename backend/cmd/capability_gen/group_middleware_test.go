package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGroupMiddlewareRetainsGeneratedRoutePrefix(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "routes.go")
	source := `package fixture
func routes(){
 g := protected.Group("/admin/widgets", RequireRoot())
 nested := g.Group("/files", guard, anotherGuard())
 empty := nested.Group("", middleware())
 empty.GET("/:id", handler)
}`
	if e := os.WriteFile(file, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	rows, e := genFromGinSource(file, "com.corex", "tenant_jwt", "/api/v1", map[string]struct{}{})
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 1 {
		t.Fatalf("unexpected generated routes: %+v", rows)
	}
	if rows[0].Protocols[0].Endpoint != "/api/v1/admin/widgets/files/:id" {
		t.Fatalf("middleware dropped group prefix: %+v", rows[0])
	}
}
