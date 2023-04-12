package model

type DataDictionaryItem struct {
	Model
	Key         string `gorm:"index:idx_key_type"`
	Type        string `gorm:"index:idx_key_type"`
	Name        string
	Value       string
	Sort        int `gorm:"default:0"`
	Description string
}

type DataDictionaryType struct {
	Model
	Type        string `gorm:"unique"`
	Name        string
	Description string
}
