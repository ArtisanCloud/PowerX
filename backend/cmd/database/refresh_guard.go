package main

import "fmt"

func validateRefreshEnvironment(env string) error {
	if env != "dev" && env != "test" {
		return fmt.Errorf("database refresh is forbidden for deployment.env=%q; use migrate for upgrades", env)
	}
	return nil
}
func validateRefreshConfirmation(database, schema string, confirm bool, confirmedDatabase, confirmedSchema string) error {
	if !confirm || confirmedDatabase != database || confirmedSchema != schema {
		return fmt.Errorf("refresh requires -confirm -confirm-database %s -confirm-schema %s; this deletes the entire schema", database, schema)
	}
	return nil
}
