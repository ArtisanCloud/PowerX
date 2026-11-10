package media

import (
	"encoding/hex"
	"errors"
	"github.com/google/uuid"
	"strings"
)

var errBackfillPlanChanged = errors.New("MEDIA_BACKFILL_PLAN_CHANGED")

// Valid checks the source fence and exact, named scope of an operator plan.
func (e ImageDerivativeBackfillExpectation) Valid() bool {
	raw, err := hex.DecodeString(e.SourceChecksum)
	if !canonicalBackfillTenant(e.TenantUUID) || err != nil || len(raw) != 32 || strings.ToLower(e.SourceChecksum) != e.SourceChecksum || len(e.Missing) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, name := range e.Missing {
		if seen[name] || (name != "preview" && name != "thumbnail" && name != "thumbnail_64") {
			return false
		}
		seen[name] = true
	}
	return true
}

func canonicalBackfillTenant(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}

func (e ImageDerivativeBackfillExpectation) allows(missing []string) bool {
	for _, name := range missing {
		found := false
		for _, approved := range e.Missing {
			if name == approved {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
