package config

type Database struct {
	Driver           string
	DSN              string
	SeedCommerceData bool
	SeedDepartment   bool
}
