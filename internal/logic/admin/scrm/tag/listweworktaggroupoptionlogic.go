package tag

import (
	"PowerX/internal/model/scrm/tag"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkTagGroupOptionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkTagGroupOptionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkTagGroupOptionLogic {
	return &ListWeWorkTagGroupOptionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListWeWorkTagGroupOption
//
//	@Description:
//	@receiver group
//	@return resp
//	@return err
func (group *ListWeWorkTagGroupOptionLogic) ListWeWorkTagGroupOption() (resp *types.ListWeWorkTagGroupReply, err error) {

	reply, _ := group.svcCtx.PowerX.SCRM.Wechat.FindListWeWorkTagGroupOption()

	return &types.ListWeWorkTagGroupReply{
		List: group.DTO(reply),
	}, err

}

// DTO
//
//	@Description:
//	@receiver group
//	@param groups
//	@return obj
func (group *ListWeWorkTagGroupOptionLogic) DTO(groups []*tag.WeWorkTagGroup) (obj []*types.WeWorkTagGroup) {

	if groups != nil {
		for _, g := range groups {
			obj = append(obj, &types.WeWorkTagGroup{
				GroupId:   g.GroupId,
				GroupName: g.Name,
			})
		}
	}
	return obj

}
