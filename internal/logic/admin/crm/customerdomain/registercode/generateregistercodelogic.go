package registercode

import (
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/pkg/stringx"
	"context"
	"sync"

	"github.com/zeromicro/go-zero/core/logx"
)

type GenerateRegisterCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGenerateRegisterCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GenerateRegisterCodeLogic {
	return &GenerateRegisterCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GenerateRegisterCodeLogic) GenerateRegisterCode(req *types.GenerateRegisterCodeRequest) (resp *types.GenerateRegisterCodeReply, err error) {

	if req.BatchCount <= 0 {
		req.BatchCount = 3
	}

	if req.BatchCount > 500 {
		req.BatchCount = 500
	}

	registerCodes := TransformRequestToBatchGenerateRegisterCode(req.BatchCount)
	//fmt.Dump(registerCodes)
	registerCodes, err = l.svcCtx.PowerX.RegisterCode.UpsertRegisterCodes(l.ctx, registerCodes)

	return &types.GenerateRegisterCodeReply{
		Result: true,
	}, nil
}

func TransformRequestToBatchGenerateRegisterCode(batchCount int) []*customerdomain.RegisterCode {
	var wg sync.WaitGroup
	batchRegisterCodes := make([]*customerdomain.RegisterCode, batchCount)
	codeChan := make(chan string, batchCount)

	// 启动协程生成注册码
	for i := 0; i < batchCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code := GenerateRegisterCode(6)
			codeChan <- code
		}()
	}

	// 等待所有协程完成
	go func() {
		wg.Wait()
		close(codeChan)
	}()

	// 从通道中读取生成的注册码
	for i := 0; i < batchCount; i++ {
		code := <-codeChan
		batchRegisterCodes[i] = &customerdomain.RegisterCode{Code: code}
	}

	return batchRegisterCodes
}

// GenerateRegisterCode 生成指定数量的随机注册码
func GenerateRegisterCode(num int) string {
	return stringx.GenerateRandomCode(num)
}
