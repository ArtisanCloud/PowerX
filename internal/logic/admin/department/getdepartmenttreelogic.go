package department

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

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
	deps, err := l.svcCtx.PowerX.Organization.FindManyDepartmentsByRootId(l.ctx, req.DepId)
	if err != nil {
		return nil, err
	}

	// make node
	var rootNode *types.DepartmentNode
	var voSlice []types.DepartmentNode
	voGroupByPid := make(map[int64][]types.DepartmentNode)
	for _, department := range deps {
		node := types.DepartmentNode{
			Id:      department.Id,
			DepName: department.Name,
		}
		if department.Leader != nil {
			node.Leader = types.DepartmentLeader{
				Id:       department.Leader.Id,
				Name:     department.Leader.Name,
				NickName: department.Leader.NickName,
				Avatar:   department.Leader.Avatar,
			}
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
