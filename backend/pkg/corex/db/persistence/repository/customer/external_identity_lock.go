package customer

import "gorm.io/gorm"

// LockExternalIdentity serializes explicit binding/creation and login resolution
// by the same global provider+subject key. PostgreSQL releases it at tx end.
func LockExternalIdentity(tx *gorm.DB, provider, subject string) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", provider+":"+subject).Error
}
