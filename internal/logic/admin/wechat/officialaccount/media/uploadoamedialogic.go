package media

import (
	"PowerX/internal/logic/admin/mediaresource"
	"PowerX/internal/types/errorx"
	"context"
	fmt2 "fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/officialAccount/material/response"
	"io"
	"net/http"
	"os"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UploadOAMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUploadOAMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadOAMediaLogic {
	return &UploadOAMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UploadOAMediaLogic) UploadOAMedia(r *http.Request) (resp *types.CreateOAMediaReply, err error) {

	err = r.ParseMultipartForm(mediaresource.MaxFileSize)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	file, _, err := r.FormFile("file")
	//fmt.Dump(handler.Filename)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}
	defer file.Close()

	// 读取文件内容
	fileContents, err := io.ReadAll(file)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "failed to read file content")
	}

	// 获取文件的临时目录和文件名
	tempDir := os.TempDir()
	tempFileName := fmt2.Sprintf("%d_*.jpg", time.Now().Unix())

	// 创建临时文件
	tempFile, err := os.CreateTemp(tempDir, tempFileName)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "failed to create temporary file")
	}
	defer os.Remove(tempFile.Name()) // 删除临时文件，确保在函数退出时清理

	// 将文件内容保存到临时文件
	if err := os.WriteFile(tempFile.Name(), fileContents, 0644); err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "failed to save file")
	}
	//fmt.Dump("temp", tempFile.Name(), "end")
	paramValues := r.Form
	res := &response.ResponseMaterialAddMaterial{}
	_, err = l.svcCtx.PowerX.WechatOA.App.Material.Upload(l.ctx, paramValues.Get("type"), tempFile.Name(), &object.StringMap{}, res)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}
	if res.ErrCode != 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, res.ErrMsg)
	}

	return &types.CreateOAMediaReply{
		Success: true,
	}, nil
}
