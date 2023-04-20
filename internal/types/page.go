package types

import (
	"PowerX/internal/model/powermodel"
)

type PageOption[T any] struct {
	Option    T
	PageIndex int
	PageSize  int
}

func (p *PageOption[T]) DefaultPageIfNotSet() {
	if p.PageIndex == 0 {
		p.PageIndex = 1
	}
	if p.PageSize == 0 {
		p.PageSize = powermodel.PageDefaultSize
	}
}

type PageEmbedOption struct {
	PageIndex int
	PageSize  int
}

func (p *PageEmbedOption) DefaultPageIfNotSet() {
	if p.PageIndex == 0 {
		p.PageIndex = 1
	}
	if p.PageSize == 0 {
		p.PageSize = powermodel.PageDefaultSize
	}
}

type Page[T any] struct {
	List      []T
	PageIndex int
	PageSize  int
	Total     int64
}
