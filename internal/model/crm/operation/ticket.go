package operation

import (
	"PowerX/internal/model/powermodel"
)

type TicketRecord struct {
	powermodel.PowerModel

	CustomerId          int64   `gorm:"comment:客户Id; index" json:"customerId"`
	TemplateId          int64   `gorm:"comment:模板Id" json:"templateId"`
	JobID               string  `gorm:"comment:作业ID" json:"jobID"`
	Count               int     `gorm:"comment:作业生图数量" json:"count"`
	DeductedTokenAmount float64 `gorm:"comment:使用代币金额" json:"deductedAmount"`
	IsUsed              bool    `gorm:"comment:是否使用" json:"isUsed"`
}

func (mdl *TicketRecord) GetTableName(needFull bool) string {
	tableName := TableNameTicketRecord
	if needFull {
		tableName = "public." + tableName
	}
	return tableName
}

const TableNameTicketRecord = "ticket_record"

const TicketRecordUniqueId = powermodel.UniqueId
