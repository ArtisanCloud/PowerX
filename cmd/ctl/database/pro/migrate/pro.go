package migrate

import (
	"gorm.io/gorm"
)

func AutoMigratePro(db *gorm.DB) {
	// migrate your pro models

	_ = db.AutoMigrate()

}
