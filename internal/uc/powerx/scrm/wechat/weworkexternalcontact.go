package wechat

import "gorm.io/gorm"

type ExternalContractUseCase struct {
	db *gorm.DB
}

func NewFunc(db *gorm.DB) *ExternalContractUseCase {
	return &ExternalContractUseCase{
		db: db,
	}
}

func (e *ExternalContractUseCase) CreateCustomerContact() {
	// 在这里插入创建客户联系的代码
}
