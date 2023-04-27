package reservation

import (
	product2 "PowerX/internal/model/custom/product"
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/product"
	"context"
	"github.com/pkg/errors"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateReservationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

type AppointmentRequest struct {
	Schedule        *reservationcenter.Schedule
	Artisan         *product.Artisan
	Customer        *customerdomain.Customer
	ServiceSpecific *product2.ServiceSpecific
	Req             *types.CreateReservationRequest
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

	schedule, err := l.svcCtx.Custom.Schedule.GetSchedule(l.ctx, req.ScheduleId)
	if err != nil {
		return nil, err
	}

	artisan, err := l.svcCtx.PowerX.Artisan.GetArtisan(l.ctx, req.ReservedArtisanId)
	if err != nil {
		return nil, err
	}

	customer, err := l.svcCtx.PowerX.Customer.GetCustomer(l.ctx, req.CustomerId)
	if err != nil {
		return nil, err
	}
	serviceSpecific, err := l.svcCtx.Custom.ServiceSpecific.GetServiceSpecific(l.ctx, req.ServiceId)
	if err != nil {
		return nil, err
	}

	request := &AppointmentRequest{
		Schedule:        schedule,
		Artisan:         artisan,
		Customer:        customer,
		ServiceSpecific: serviceSpecific,
		Req:             req,
	}

	reservation, err := l.svcCtx.Custom.Reservation.MakeAppointment(l.ctx, request)
	if err != nil {
		return nil, err
	}

	resp = &types.CreateReservationReply{
		ReservationKey: reservation.Id,
	}

	return resp, nil
}
