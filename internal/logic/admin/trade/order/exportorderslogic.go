package order

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/trade"
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
func (l *ExportOrdersLogic) ExportOrders(req *types.ExportOrdersRequest) (resp *types.ExportOrdersReply, err error) {

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
	header := []string{"订单号", "金额", "日期"}
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

		// 逐行写入订单数据
		for _, order := range orders {
			row := []string{order.OrderNumber, fmt2.Sprintf("%f", order.UnitPrice), order.CompletedAt.String()}
			err := writer.Write(row)
			if err != nil {
				log.Fatal("无法写入CSV行:", err)
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
