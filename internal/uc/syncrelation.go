package uc

import (
	"PowerX/internal/types"
	"fmt"
)

type SyncRelation struct {
	Type           string
	SystemId       int64
	SystemUniKey   string
	Platform       string
	PlatformIdInt  int64
	PlatformIdStr  string
	PlatformUniKey string
	Value          string
	ValueHash      string
	Hash1          string
	Hash2          string
	*types.Model
}

func (s *SyncRelation) AutoFillUniKey() {
	s.SystemUniKey = fmt.Sprintf("%s_%d", s.Type, s.SystemId)
	if s.PlatformIdStr == "" {
		s.PlatformUniKey = fmt.Sprintf("%s_%s_%d", s.Type, s.Platform, s.PlatformIdInt)
	} else {
		s.PlatformUniKey = fmt.Sprintf("%s_%s_%d", s.Type, s.Platform, s.PlatformIdInt)
	}
}

const (
	PlatformSystem string = "SYSTEM"
	PlatformWeWork        = "WE_WORK"
)

const (
	RelationTypeTag string = "TAG"
)
