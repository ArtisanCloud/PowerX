package operation

import (
	"PowerX/internal/model/crm/operation"
	"context"
	"gorm.io/gorm"
)

type TicketUseCase struct {
	db *gorm.DB
}

func NewTicketUseCase(db *gorm.DB) *TicketUseCase {
	return &TicketUseCase{db: db}
}

func (uc *TicketUseCase) CreateTicket(ctx context.Context, ticket *operation.TicketRecord) error {
	result := uc.db.WithContext(ctx).Create(ticket)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (uc *TicketUseCase) GetTicketByJobID(ctx context.Context, jobId string) (*operation.TicketRecord, error) {
	ticket := &operation.TicketRecord{}
	result := uc.db.WithContext(ctx).
		//Debug().
		First(ticket, "job_id", jobId)
	if result.Error != nil {
		return nil, result.Error
	}
	return ticket, nil
}

func (uc *TicketUseCase) GetTicketByID(ctx context.Context, id int64) (*operation.TicketRecord, error) {
	ticket := &operation.TicketRecord{}
	result := uc.db.WithContext(ctx).First(ticket, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return ticket, nil
}

func (uc *TicketUseCase) UpdateTicketById(ctx context.Context, id int64, ticket *operation.TicketRecord) error {
	result := uc.db.WithContext(ctx).Model(&operation.TicketRecord{}).Where("id = ?", id).Updates(ticket)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (uc *TicketUseCase) UpdateTicketUsage(ctx context.Context, id int64, isUsed bool) error {
	result := uc.db.WithContext(ctx).Model(&operation.TicketRecord{}).Where("id = ?", id).Update("is_used", isUsed)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (uc *TicketUseCase) GetTicketsByStatus(ctx context.Context, customerId int64, isUsed bool) ([]*operation.TicketRecord, error) {
	tickets := []*operation.TicketRecord{}
	result := uc.db.WithContext(ctx).Find(&tickets, "customer_id = ? AND is_used = ?", customerId, isUsed)
	if result.Error != nil {
		return nil, result.Error
	}
	return tickets, nil
}
