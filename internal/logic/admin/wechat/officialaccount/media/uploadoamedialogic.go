package media

import (
	"PowerX/internal/logic/admin/mediaresource"
	"PowerX/internal/types/errorx"
	fmt "PowerX/pkg/printx"
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

	// 解析表单数据
	err = r.ParseMultipartForm(mediaresource.MaxFileSize)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, err.Error())
	}

	paramValues := r.Form
	mediaType := paramValues.Get("type")
	fmt.Dump(mediaType)
	query := &object.StringMap{}
	res := &response.ResponseMaterialAddMaterial{}
	if mediaType == "video" {
		jsonDescription, err := object.JsonEncode(&object.HashMap{
			"title":        paramValues.Get("title"),
			"introduction": paramValues.Get("description"),
		})
		if err != nil {
			return nil, err
		}

		query = &object.StringMap{
			"Description": jsonDescription,
		}
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
	if mediaType == "video" {
		tempFileName = fmt2.Sprintf("%d_*.mp4", time.Now().Unix())
	} else if mediaType == "voice" {
		tempFileName = fmt2.Sprintf("%d_*.mp3", time.Now().Unix())
	}

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

	_, err = l.svcCtx.PowerX.WechatOA.App.Material.Upload(l.ctx, mediaType, tempFile.Name(), query, res)
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
