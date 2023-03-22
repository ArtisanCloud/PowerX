package department

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/zeromicro/go-zero/core/logx"
)

type GetDepartmentTreeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDepartmentTreeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDepartmentTreeLogic {
	return &GetDepartmentTreeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDepartmentTreeLogic) GetDepartmentTree(req *types.GetDepartmentTreeRequest) (resp *types.GetDepartmentTreeReply, err error) {
	var userIds []int64
	depPage := l.svcCtx.UC.Department.FindManyDepartments(l.ctx, &powerx.FindManyDepartmentsOption{
		RootId: req.DepId,
	})

	if depPage.Total == 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到根部门")
	}

	for _, department := range depPage.List {
		for _, id := range department.LeaderIds {
			userIds = append(userIds, id)
		}
	}

	// make node
	var rootNode *types.DepartmentNode
	var voSlice []types.DepartmentNode
	voGroupByPid := make(map[int64][]types.DepartmentNode)
	for _, department := range depPage.List {
		node := types.DepartmentNode{
			Id:        department.ID,
			DepName:   department.Name,
			LeaderIds: department.LeaderIds,
		}
		voSlice = append(voSlice, node)
		if node.Id == req.DepId {
			rootNode = &node
		}
		if voGroupByPid[department.PId] == nil {
			voGroupByPid[department.PId] = make([]types.DepartmentNode, 0, 1)
		}
		voGroupByPid[department.PId] = append(voGroupByPid[department.PId], node)
	}

	if rootNode == nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到根部门")
	}

	var makeChildren func(currentNode types.DepartmentNode) []types.DepartmentNode
	makeChildren = func(currentNode types.DepartmentNode) []types.DepartmentNode {
		if nodes, ok := voGroupByPid[currentNode.Id]; ok {
			for i := range nodes {
				nodes[i].Children = makeChildren(nodes[i])
			}
			return nodes
		} else {
			return make([]types.DepartmentNode, 0)
		}
	}
	rootNode.Children = makeChildren(*rootNode)
	resp = &types.GetDepartmentTreeReply{
		DepTree: *rootNode,
	}
	return
}
