package models

type DataDictionary struct {
	*Model
	Key         string `gorm:"uniqueIndex"`
	Value       string
	Description string
	Type        string `gorm:"index"`
}
