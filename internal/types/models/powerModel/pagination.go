package powerModel

import "gorm.io/gorm"

type Pagination struct {
	Limit      int         `json:"limit"`
	Page       int         `json:"page"`
	Sort       string      `json:"sort"`
	TotalRows  int64       `json:"totalRows"`
	TotalPages int         `json:"totalPages"`
	Data       interface{} `json:"data"`
}

func NewPagination(page int, limit int, sort string) *Pagination {

	p := &Pagination{}
	p.SetPage(page)
	p.SetLimit(limit)
	p.SetSort(sort)

	return p

}

/**
 * Pagination
 */
func Paginate(page int, pageSize int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if page == 0 {
			page = 1
		}

		switch {
		case pageSize > 100:
			pageSize = 100
		case pageSize <= 0:
			pageSize = 10
		}

		offset := (page - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}

func (p *Pagination) GetOffset() int {
	return (p.GetPage() - 1) * p.GetLimit()
}

func (p *Pagination) SetLimit(limit int) *Pagination {

	if p.Limit <= 0 {
		p.Limit = 10
	}

	p.Limit = limit
	return p
}
func (p *Pagination) GetLimit() int {
	if p.Limit == 0 {
		p.Limit = 10
	}
	return p.Limit
}

func (p *Pagination) SetPage(page int) *Pagination {
	if p.Page <= 0 {
		p.Page = 1
	}

	p.Page = page

	return p
}

func (p *Pagination) GetPage() int {
	if p.Page <= 0 {
		p.Page = 1
	}
	return p.Page
}

func (p *Pagination) SetSort(sort string) *Pagination {

	p.Sort = sort
	return p
}

func (p *Pagination) GetSort() string {
	return p.Sort
}
