package reservation

import (
	"PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"
	"github.com/golang-module/carbon/v2"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"sync"
)

const (
	ReservationChannelBuff = 100
)

type AppointmentResponse struct {
	// *types.CreateReservationRequest，用于接收预约请求。
	*types.CreateReservationReply
	Error error
}

type CreateReservationByChanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext

	// ReservationChannel：预约请求通道，类型为 chan
	ReservationChannel chan *AppointmentRequest

	// 结果通道，类型为 chan *AppointmentResponse，用于返回处理预约请求的结果。
	ResultChannel chan *AppointmentResponse

	// wg：sync.WaitGroup 类型，用于等待所有的处理预约请求的 goroutine 完成。
	wg sync.WaitGroup
}

func NewCreateReservationByChanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateReservationByChanLogic {
	reservationChannel := make(chan *AppointmentRequest, ReservationChannelBuff)
	resultChan := make(chan *AppointmentResponse)
	l := &CreateReservationByChanLogic{
		Logger:             logx.WithContext(ctx),
		ctx:                ctx,
		svcCtx:             svcCtx,
		ReservationChannel: reservationChannel,
		ResultChannel:      resultChan,
	}

	// 创建了 10 个 goroutine，
	// 每个 goroutine 都会执行 ProcessAppointmentRequests 函数来处理预约请求，
	// 同时将 wg 加 1，表示有一个 goroutine 开始执行。
	for i := 0; i < 10; i++ {
		l.wg.Add(1)
		go l.ProcessAppointmentRequests()
	}

	return l
}

// 在 CreateReservation 中，会将请求放入 ReservationChannel 中，并等待结果返回，
// 同时会判断是否达到了请求通道的最大容量，如果达到了，则返回错误信息。
func (l *CreateReservationByChanLogic) CreateReservation(req *types.CreateReservationRequest) (resp *types.CreateReservationReply, err error) {

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

	//customer, err := l.svcCtx.PowerX.Customer.GetCustomer(l.ctx, req.CustomerId)
	//if err != nil {
	//	return nil, err
	//}
	serviceSpecific, err := l.svcCtx.Custom.ServiceSpecific.GetServiceSpecific(l.ctx, req.ServiceId)
	if err != nil {
		return nil, err
	}

	request := &AppointmentRequest{
		Schedule: schedule,
		Artisan:  artisan,
		//Customer: customer,
		ServiceSpecific: serviceSpecific,
		req:             req,
	}

	select {
	case l.ReservationChannel <- request:
		return l.waitForResult()
	default:
		return nil, errors.New("appointment queue is full")
	}
}

// 在 waitForResult 中，会从 ResultChannel 中等待处理请求的结果返回。
// 如果返回的结果包含了预约的 id，则表示预约操作成功，否则表示操作失败。
func (l *CreateReservationByChanLogic) waitForResult() (*types.CreateReservationReply, error) {
	select {
	case result := <-l.ResultChannel:
		if result.CreateReservationReply != nil && result.CreateReservationReply.ReservationKey > 0 {
			return result.CreateReservationReply, nil
		} else {
			return nil, errors.Wrap(result.Error, "failed to create reservation")
		}
	case <-l.ctx.Done():
		return nil, l.ctx.Err()
	}
}

// 在 ProcessAppointmentRequests 中，会不断地从 ReservationChannel 中接收请求。
// 接收到请求后，会将请求转换为一个 reservationcenter.Reservation 对象，然后执行预约操作。
// 如果预约操作失败，则会将错误信息放入 ResultChannel 中返回。
// 否则，将预约的 id 放入 ResultChannel 中返回。
func (l *CreateReservationByChanLogic) ProcessAppointmentRequests() {

	// 注意，在 ProcessAppointmentRequests 中，
	// 由于需要使用 goroutine 来并发地处理预约请求，因此使用了 sync.WaitGroup 来等待所有的 goroutine 完成。
	// 在函数的最后，使用了 wg.Done() 来表示一个 goroutine 完成了处理任务。
	defer l.wg.Done()

	for {
		select {
		case req := <-l.ReservationChannel:
			// 处理预约请求
			isAvailable, usedTimeResource := l.svcCtx.Custom.Schedule.IsScheduleAvailable(l.ctx, req.Schedule, req.Artisan, req.ServiceSpecific)
			//fmt.Dump(isAvailable, usedTimeResource)
			if isAvailable {
				// bucket的开始时间为基准，加上占用的分钟数，为该预约记录的预约时间
				reservedTime := carbon.Time2Carbon(req.Schedule.StartTime).AddMinutes(usedTimeResource)
				//fmt.Dump(reservedTime)
				operationStatus := l.svcCtx.PowerX.DataDictionary.GetCachedDD(l.ctx, reservationcenter.OperationStatusType, reservationcenter.OperationStatusNone)
				reservationStatus := l.svcCtx.PowerX.DataDictionary.GetCachedDD(l.ctx, reservationcenter.ReservationStatusType, reservationcenter.ReservationStatusConfirmed)

				// 可以建立预约记录
				reservation := &reservationcenter.Reservation{
					ScheduleId:        req.req.ScheduleId,
					CustomerId:        req.req.CustomerId,
					ReservedArtisanId: req.req.ReservedArtisanId,
					ServiceId:         req.req.ServiceId,
					ServiceDuration:   req.ServiceSpecific.MandatoryDuration,
					SourceChannelId:   req.req.SourceChannelId,
					Type:              req.req.Type,
					ReservedTime:      reservedTime.ToStdTime(),
					Description:       req.req.Description,
					ConsumedPoints:    req.req.ConsumedPoints,
					OperationStatus:   operationStatus,
					ReservationStatus: reservationStatus,
				}
				// 创建预约记录
				err := l.svcCtx.Custom.Reservation.CreateReservation(l.ctx, reservation)
				if err != nil {
					l.Errorf("failed to create reservation, %v", err)
					result := &AppointmentResponse{
						Error: err,
					}
					l.ResultChannel <- result
				} else {
					result := &AppointmentResponse{
						&types.CreateReservationReply{
							ReservationKey: reservation.Id,
						}, nil,
					}
					l.ResultChannel <- result
				}
			} else {
				result := &AppointmentResponse{
					Error: errors.New("当前预约请求无效"),
				}
				l.ResultChannel <- result
			}

		case <-l.ctx.Done():
			return
		}
	}
}
