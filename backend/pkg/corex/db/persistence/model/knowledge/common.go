package knowledge

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

func tableName(name string) string {
	schema := strings.TrimSpace(coremodel.PowerXSchema)
	if schema == "" {
		return name
	}
	return schema + "." + name
}
