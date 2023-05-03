package reservation

import (
	"PowerX/internal/uc/custom/reservationcenter"
	"context"
	"github.com/pkg/errors"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateReservationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateReservationLogic {
	return &CreateReservationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateReservationLogic) CreateReservation(req *types.CreateReservationRequest) (resp *types.CreateReservationReply, err error) {

	pivot, _ := l.svcCtx.Custom.Schedule.LoadPivotScheduleToArtisan(l.ctx, req.ScheduleId, req.ReservedArtisanId)
	if pivot != nil && pivot.IsAvailable {
		return nil, errors.New("发型师已经被约满")
	}

	// 判断该schedule是否已经过了预约时间
	schedule, err := l.svcCtx.Custom.Schedule.GetSchedule(l.ctx, req.ScheduleId)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	//fmt.Dump(now, schedule.StartTime)
	if schedule.StartTime.Before(now) {
		return nil, errors.New("该时间段已经过了可预约时间")
	}

	// 判断是否该用户在在时段已经预约过
	customer, err := l.svcCtx.PowerX.Customer.GetCustomer(l.ctx, req.CustomerId)
	if err != nil {
		return nil, err
	}
	reservedRecords, _ := l.svcCtx.Custom.Reservation.GetReservedRecordsBy(l.ctx, customer, schedule)
	if len(reservedRecords) > 0 {
		return nil, errors.New("该时间段同一客户不能重复预约")
	}

	artisan, err := l.svcCtx.PowerX.Artisan.GetArtisan(l.ctx, req.ReservedArtisanId)
	if err != nil {
		return nil, err
	}

	serviceSpecific, err := l.svcCtx.Custom.ServiceSpecific.GetServiceSpecific(l.ctx, req.ServiceId)
	if err != nil {
		return nil, err
	}

	request := &reservationcenter.AppointmentRequest{
		Schedule:        schedule,
		Artisan:         artisan,
		Customer:        customer,
		ServiceSpecific: serviceSpecific,
		Req:             req,
	}

	reservation, err := l.svcCtx.Custom.Schedule.MakeAppointment(l.ctx, request)
	if err != nil {
		return nil, err
	}

	resp = &types.CreateReservationReply{
		ReservationKey: reservation.Id,
	}

	return resp, nil
}
