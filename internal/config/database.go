package config

type Database struct {
	Driver           string
	DSN              string
	Schema           string
	SeedCommerceData bool
	SeedDepartment   bool
}
