package order

import (
	"PowerX/internal/model/crm/trade"
	"PowerX/internal/types/errorx"
	trade2 "PowerX/internal/uc/powerx/crm/trade"
	"context"
	"encoding/csv"
	"io"
	"net/http"
	"sync"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

const MaxFileSize = 2 << 20

type ImportOrdersLogic struct {
	logx.Logger
	ctx                      context.Context
	svcCtx                   *svc.ServiceContext
	OrderStatusToBeShippedId int
	OrderStatusShippingId    int
	orderProcessingCh        chan *trade.Order
	OrdersIgnored            []string
	OrdersFailed             []*trade.Order
	OrdersSucceeded          []*trade.Order
}

func NewImportOrdersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ImportOrdersLogic {
	return &ImportOrdersLogic{
		Logger:            logx.WithContext(ctx),
		ctx:               ctx,
		svcCtx:            svcCtx,
		orderProcessingCh: make(chan *trade.Order, 100)}
}

func (l *ImportOrdersLogic) ImportOrders(r *http.Request) (resp *types.ImportOrdersReply, err error) {
	// 获取上传文件
	err = r.ParseMultipartForm(MaxFileSize)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	file, _, err := r.FormFile("resource")
	//fmt.Dump(handler.Filename)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}
	defer file.Close()

	// 批量扫描csv文件的行数据，并行筛选订单
	reader := csv.NewReader(file)
	var wg sync.WaitGroup
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// 增加 WaitGroup 计数
		wg.Add(1)
		go func(record []string) {
			defer wg.Done() // 减少 WaitGroup 计数

			//fmt2.Dump(record)
			// 处理单行数据
			l.processCSVRow(record)
		}(record)
	}

	// ... 等待所有并行处理完成 ...
	wg.Wait()

	// 关闭订单处理通道
	close(l.orderProcessingCh)

	// 启动 Goroutine 处理订单
	l.processOrders()

	totalSucceeded := len(l.OrdersSucceeded)
	totalIgnored := len(l.OrdersIgnored)
	totalFailed := len(l.OrdersFailed)
	total := totalSucceeded + totalIgnored + totalFailed
	return &types.ImportOrdersReply{
		Total:   total,
		Success: totalSucceeded,
		Ignored: totalIgnored,
		Failed:  totalFailed,
	}, nil
}

func (l *ImportOrdersLogic) processCSVRow(record []string) {

	if record[0] != "" && record[11] != "" {
		orderNo := record[0]
		trackingCode := record[11]

		if orderNo == "订单号" {
			return
		}

		//fmt2.Dump(orderNo, trackingCode)
		result, _ := l.svcCtx.PowerX.Order.FindManyOrders(context.Background(), &trade2.FindManyOrdersOption{
			OrderNumbers: []string{orderNo},
			Status:       []int{l.OrderStatusToBeShippedId},
			PageEmbedOption: types.PageEmbedOption{
				PageSize: 1,
			},
		})

		//fmt2.Dump(len(result.List))

		if len(result.List) > 0 {
			order := result.List[0]
			//fmt2.Dump(order.OrderNumber)
			if order.Logistics == nil {
				order.Logistics = &trade.Logistics{
					OrderId: order.Id,
				}
			}
			order.Logistics.TrackingCode = trackingCode

			l.orderProcessingCh <- order
		} else {
			l.OrdersIgnored = append(l.OrdersIgnored, orderNo)
		}

	}

}

func (l *ImportOrdersLogic) processOrders() {

	var wg sync.WaitGroup
	numWorkers := 5

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			//fmt2.Dump(fmt.Sprintf("workId-%d", workerID))
			for order := range l.orderProcessingCh {
				//fmt.Printf("Updated order %s, tracking to %s\n", order.OrderNumber, order.Logistics.TrackingCode)

				order.Status = l.OrderStatusShippingId
				order.UpdatedAt = time.Now()
				order, err := l.svcCtx.PowerX.Order.UpsertOrderWithLogistic(context.Background(), order)
				if err != nil {
					l.OrdersFailed = append(l.OrdersFailed, order)
				} else {
					l.OrdersSucceeded = append(l.OrdersSucceeded, order)
				}
			}
		}(i)
	}

	// 等待所有并发处理订单的 Goroutine 完成
	wg.Wait()
}
