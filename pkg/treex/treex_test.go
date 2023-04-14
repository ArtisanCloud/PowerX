package treex

import (
	fmt "PowerX/pkg/printx"
	"testing"
)

type MyNode struct {
	Id          int64
	PId         int64
	Name        string
	Sort        int
	ViceName    string
	Description string
	Children    []MyNode
}

func TestBuildTree(t *testing.T) {
	// 构造数据
	data := []MyNode{
		{Id: 1, PId: 0, Name: "电子产品", Sort: 1, ViceName: "电子产品副标题", Description: "电子产品描述"},
		{Id: 2, PId: 1, Name: "手机", Sort: 1, ViceName: "手机副标题", Description: "手机描述"},
		{Id: 3, PId: 1, Name: "电脑", Sort: 2, ViceName: "电脑副标题", Description: "电脑描述"},
		{Id: 4, PId: 2, Name: "小米手机", Sort: 1, ViceName: "小米手机副标题", Description: "小米手机描述"},
		{Id: 5, PId: 2, Name: "华为手机", Sort: 2, ViceName: "华为手机副标题", Description: "华为手机描述"},
		{Id: 6, PId: 3, Name: "苹果电脑", Sort: 1, ViceName: "苹果电脑副标题", Description: "苹果电脑描述"},
		{Id: 7, PId: 3, Name: "华硕电脑", Sort: 2, ViceName: "华硕电脑副标题", Description: "华硕电脑描述"},
	}

	// 转换成树形结构
	tree := BuildTree(data,
		0,
		func(node MyNode) int64 { return node.Id },
		func(node MyNode) int64 { return node.PId },
		func(parentNode MyNode, childrenNodes []MyNode) MyNode {
			parentNode.Children = childrenNodes
			return parentNode
		},
	)

	fmt.Dump(tree)

	// 输出树形结构
	PrintTree(tree, 0, func(node MyNode) []MyNode {
		return node.Children
	})
}
