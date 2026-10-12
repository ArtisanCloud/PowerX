package main

import "testing"

func TestRefreshGuard(t *testing.T) {
	for _, env := range []string{"prod", "staging", "", "production"} {
		if validateRefreshEnvironment(env) == nil {
			t.Fatalf("unsafe environment allowed: %s", env)
		}
	}
	for _, env := range []string{"dev", "test"} {
		if err := validateRefreshEnvironment(env); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		confirmed        bool
		database, schema string
	}{{false, "powerx_test", "public"}, {true, "powerx_pro", "public"}, {true, "powerx_test", "other"}} {
		if validateRefreshConfirmation("powerx_test", "public", c.confirmed, c.database, c.schema) == nil {
			t.Fatal("unsafe confirmation accepted")
		}
	}
	if err := validateRefreshConfirmation("powerx_test", "public", true, "powerx_test", "public"); err != nil {
		t.Fatal(err)
	}
}
