package order

import (
	trade2 "PowerX/internal/model/trade"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/crm/trade"
	"context"
	"encoding/csv"
	"errors"
	fmt2 "fmt"
	"github.com/golang-module/carbon/v2"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

var ExcelDataFields = map[string][]string{
	"headers": []string{
		"订单号",
		"客户名称",
		"产品名称",
		"金额",
		"SKU",
		"数量",
		"订单类型",
		"订单状态",
		"收货人",
		"收货人电话",
		"收货地址",
		"物流追踪号",
		//"物流状态",
		"物流承运商",
		"预计送达时间",
		"实际送达时间",
		"订单创建日期",
	},
	"fields": []string{
		"orders.order_number",
		"customers.name",
		"order_items.product_name",
		"order_items.unit_price",
		"order_items.sku_no",
		"order_items.quantity",
		"orders.type",
		"orders.status",
		"delivery_addresses.recipient",
		"delivery_addresses.phone_number",
		"delivery_addresses.address_line",
		"logistics.tracking_code",
		//"logistics.status",
		"logistics.carrier",
		"logistics.estimated_delivery_date",
		"logistics.actual_delivery_date",
		"orders.created_at",
	},
}

type ExportOrdersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewExportOrdersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ExportOrdersLogic {
	return &ExportOrdersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}
func (l *ExportOrdersLogic) ExportOrders(ctx context.Context, req *types.ExportOrdersRequest) (resp *types.ExportOrdersReply, err error) {

	// 检查时间段是否在一个月内
	startAt := carbon.Parse(req.StartAt).ToStdTime()
	endAt := carbon.Parse(req.EndAt).ToStdTime()

	if err = CheckTimeRangeIsValid(startAt, endAt); err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	// 创建导出文件目录
	exportDir := "temp/export/order/"
	err = os.MkdirAll(exportDir, os.ModePerm)
	if err != nil {
		return nil, err
	}

	// 创建CSV文件
	exportName := GetExportNameBy(startAt, endAt)
	exportFilePath := filepath.Join(exportDir, exportName)
	file, err := os.Create(exportFilePath)
	if err != nil {
		log.Fatal("无法创建导出文件:", err)
	}
	defer file.Close()

	// 创建CSV writer
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	header := ExcelDataFields["headers"]
	err = writer.Write(header)
	if err != nil {
		log.Fatal("无法写入CSV表头:", err)
	}

	// 批次获取订单流，并且写入文件=
	batchSize := 100
	offset := 0
	for {
		res, err := l.svcCtx.PowerX.Order.FindManyOrders(l.ctx, &trade.FindManyOrdersOption{
			StartAt: startAt,
			EndAt:   endAt,
			PageEmbedOption: types.PageEmbedOption{
				PageIndex: offset,
				PageSize:  batchSize,
			},
		})
		if err != nil {
			return nil, errorx.WithCause(errorx.ErrNotFoundObject, err.Error())
		}

		orders := res.List
		if len(orders) == 0 {
			break // 数据读取完毕，退出循环
		}

		ucDD := l.svcCtx.PowerX.DataDictionary

		// 逐行写入订单数据
		for _, order := range orders {
			if len(order.Items) > 0 {
				orderType := ucDD.GetCachedDDById(ctx, order.Type).Name
				orderStatus := ucDD.GetCachedDDById(ctx, order.Status).Name
				orderRow := transformRowByOrder(order, orderType, orderStatus)
				err := writer.Write(orderRow)
				if err != nil {
					log.Fatal("订单记录无法写入CSV行:", err)
				}
				for _, orderItem := range order.Items {

					// 订单的基础信息
					itemRows := transformRowByOrderItem(order, orderItem)
					err := writer.Write(itemRows)
					if err != nil {
						log.Fatal("订单详细记录无法写入CSV行:", err)
					}
				}

			}

		}

		writer.Flush() // 刷新缓冲区，将数据写入文件

		offset += batchSize
	}

	// 读取导出文件的内容
	exportData, err := os.ReadFile(exportFilePath)
	if err != nil {
		return nil, err
	}

	// 将导出文件内容写入HTTP响应体
	resp = &types.ExportOrdersReply{
		Content:  exportData,
		FileName: exportName,
		FileSize: len(exportData),
		FileType: "text/csv",
	}

	return resp, nil
}

func CheckTimeRangeIsValid(from time.Time, to time.Time) error {
	// 获取起始时间的年、月、日
	fromYear, fromMonth, fromDay := from.Date()

	// 获取结束时间的年、月、日
	toYear, toMonth, toDay := to.Date()

	// 计算起始时间和结束时间相差的月份
	monthsDiff := (toYear-fromYear)*12 + int(toMonth) - int(fromMonth)

	// 检查月份差是否大于1
	if monthsDiff > 1 {
		return errors.New("时间范围超过一个月")
	}

	// 检查起始时间和结束时间是否在同一个月
	if fromYear == toYear && fromMonth == toMonth {
		// 检查日期差是否大于一个月
		if toDay-fromDay > 30 {
			return errors.New("时间范围超过一个月")
		}
	}

	return nil
}

func GetExportNameBy(from time.Time, to time.Time) string {
	return "export_" + from.Format("20060102") + "_to_" + to.Format("20060102") + ".csv"
}

func transformRowByOrder(order *trade2.Order, orderType string, orderStatus string) []string {
	row := []string{
		order.OrderNumber,
	}

	if order.Customer != nil {
		row = append(row, order.Customer.Name)
	} else {
		row = append(row, "")
	}

	row = append(row, "", "", "", "",
		orderType,
		orderStatus,
	)

	// 订单的收货信息
	if order.DeliveryAddress != nil {
		row = append(row, order.DeliveryAddress.Recipient,
			order.DeliveryAddress.PhoneNumber,
			order.DeliveryAddress.AddressLine,
		)
	} else {
		row = append(row, "", "", "")
	}

	// 订单的收货信息
	if order.Logistics != nil {
		row = append(row, order.Logistics.TrackingCode,
			order.Logistics.Carrier,
			order.Logistics.EstimatedDeliveryDate.String(),
			order.Logistics.ActualDeliveryDate.String(),
		)
	} else {
		row = append(row, "", "", "", "")
	}

	// 订单生成时间
	row = append(row, order.CreatedAt.String())

	return row
}

func transformRowByOrderItem(order *trade2.Order, orderItem *trade2.OrderItem) []string {

	// 订单号
	row := []string{
		"",
	}

	// 客户名称
	row = append(row, "")

	// 产品子项信息
	row = append(row, orderItem.ProductName,
		fmt2.Sprintf("%.2f", orderItem.UnitPrice),
		orderItem.SkuNo,
		fmt2.Sprintf("%d", orderItem.Quantity),
		"",
		"",
	)

	// 订单的收货信息
	row = append(row, "", "", "")

	// 订单的收货信息
	row = append(row, "", "", "", "")

	// 订单生成时间
	row = append(row, orderItem.CreatedAt.String())

	return row
}
