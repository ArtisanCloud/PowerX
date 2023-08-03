package tag

import (
	"PowerX/internal/model/scrm/tag"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkTagGroupPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkTagGroupPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkTagGroupPageLogic {
	return &ListWeWorkTagGroupPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// ListWeWorkTagGroupPage
//  @Description: 标签组分页
//  @receiver group
//  @param req
//  @return resp
//  @return err
//
func (group *ListWeWorkTagGroupPageLogic) ListWeWorkTagGroupPage(opt *types.ListWeWorkTagGroupPageRequest) (resp *types.ListWeWorkTagGroupPageReply, err error) {

	reply, err := group.svcCtx.PowerX.SCRM.Wechat.FindListWeWorkTagGroupPage(group.OPT(opt))

	return &types.ListWeWorkTagGroupPageReply{
		List:      group.DTO(reply.List),
		PageIndex: reply.PageIndex,
		PageSize:  reply.PageSize,
		Total:     reply.Total,
	}, err

}

//
//
//  @Description:
//  @receiver group
//  @param opt
//  @return *types.PageOption[types.ListWeWorkTagGroupPageRequest]
//
func (group *ListWeWorkTagGroupPageLogic) OPT(opt *types.ListWeWorkTagGroupPageRequest) *types.PageOption[types.ListWeWorkTagGroupPageRequest] {

	option := types.PageOption[types.ListWeWorkTagGroupPageRequest]{
		Option: types.ListWeWorkTagGroupPageRequest{
			GroupId:   opt.GroupId,
			GroupName: opt.GroupName,
		},
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
	}
	option.DefaultPageIfNotSet()

	return &option
}

//
// DTO
//  @Description:
//  @receiver group
//  @param datas
//  @return groups
//
func (group *ListWeWorkTagGroupPageLogic) DTO(datas []*tag.WeWorkTagGroup) (groups []*types.GroupWithTag) {

	if datas == nil {
		return groups
	}
	for _, data := range datas {
		groups = append(groups, &types.GroupWithTag{
			GroupId:   data.GroupId,
			GroupName: data.Name,
			Tags:      group.tags(data.WeWorkGroupTags),
		})
	}
	return groups

}

//
// tags
//  @Description:
//  @receiver group
//  @param datas
//  @return tags
//
func (group *ListWeWorkTagGroupPageLogic) tags(datas []*tag.WeWorkTag) (tags []*types.Tag) {

	if datas == nil {
		return tags
	}
	for _, data := range datas {
		tags = append(tags, &types.Tag{
			IsSelf: data.IsSelf,
			TagId:  data.TagId,
			Name:   data.Name,
			Sort:   data.Sort,
		})
	}
	return tags

}
