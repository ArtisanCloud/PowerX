package tag

import (
	"PowerX/internal/model/scrm/tag"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkTagPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkTagPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkTagPageLogic {
	return &ListWeWorkTagPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListWeWorkTagPage
//
//	@Description:
//	@receiver tag
//	@param req
//	@return resp
//	@return err
func (tag *ListWeWorkTagPageLogic) ListWeWorkTagPage(opt *types.ListWeWorkTagReqeust) (resp *types.ListWeWorkTagReply, err error) {

	reply, err := tag.svcCtx.PowerX.SCRM.Wechat.FindListWeWorkTagPage(tag.OPT(opt))
	if err != nil {
		return nil, err
	}

	return &types.ListWeWorkTagReply{
		List:      tag.DTO(reply.List),
		PageIndex: reply.PageIndex,
		PageSize:  reply.PageSize,
		Total:     reply.Total,
	}, err

}

// @Description:
// @receiver tag
// @param opt
// @return *types.PageOption[types.ListWeWorkTagReqeust]
func (tag *ListWeWorkTagPageLogic) OPT(opt *types.ListWeWorkTagReqeust) *types.PageOption[types.ListWeWorkTagReqeust] {

	option := types.PageOption[types.ListWeWorkTagReqeust]{
		Option:    types.ListWeWorkTagReqeust{},
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
	}
	option.DefaultPageIfNotSet()
	if len(opt.TagIds) > 0 {
		option.Option.TagIds = opt.TagIds
	}
	if len(opt.GroupIds) > 0 {
		option.Option.GroupIds = opt.GroupIds
	}
	if opt.Name != `` {
		option.Option.Name = opt.Name
	}
	return &option

}

// DTO
//
//	@Description:
//	@receiver tag
//	@param tags
//	@return obj
func (tag *ListWeWorkTagPageLogic) DTO(tags []*tag.WeWorkTag) (obj []*types.Tag) {

	if tags != nil {
		for _, val := range tags {
			obj = append(obj, &types.Tag{
				Type:      val.Type,
				IsSelf:    val.IsSelf,
				TagId:     val.TagId,
				GroupId:   val.GroupId,
				GroupName: val.WeWorkGroup.Name,
				Name:      val.Name,
				Sort:      val.Sort,
			})
		}
	}
	return obj

}
