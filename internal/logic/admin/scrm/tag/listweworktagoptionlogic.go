package tag

import (
	"PowerX/internal/model/scrm/tag"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkTagOptionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkTagOptionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkTagOptionLogic {
	return &ListWeWorkTagOptionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// ListWeWorkTagOption
//  @Description:
//  @receiver tag
//  @return resp
//  @return err
//
func (tag *ListWeWorkTagOptionLogic) ListWeWorkTagOption() (resp *types.ListWeWorkTagOptionReply, err error) {

	reply, err := tag.svcCtx.PowerX.SCRM.Wechat.FindListWeWorkTagOption()
	return &types.ListWeWorkTagOptionReply{
		List: tag.DTO(reply),
	}, err
}

//
// DTO
//  @Description:
//  @receiver tag
//  @param opt
//  @return tags
//
func (tag *ListWeWorkTagOptionLogic) DTO(opt []*tag.WeWorkTag) (tags map[string]*types.Tag) {

	tags = make(map[string]*types.Tag)
	if opt == nil {
		return nil
	}
	for _, val := range opt {
		tags[val.TagId] = &types.Tag{
			Type:      val.Type,
			TagId:     val.TagId,
			GroupId:   val.GroupId,
			GroupName: val.WeWorkGroup.Name,
			Name:      val.Name,
			Sort:      val.Sort,
		}
	}
	return tags

}
