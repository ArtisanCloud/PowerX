package treex

import (
	"errors"
	"fmt"
)

type Node[T any] struct {
	Elem     T
	Children []Node[T]
}

func MakeTree[T any, K comparable](nodes []T, getId func(T) K, getPId func(T) K, rootId K) (*Node[T], error) {
	mapById := make(map[K]T)
	mapByPId := make(map[K][]T)
	for _, node := range nodes {
		id := getId(node)
		pId := getPId(node)
		mapById[id] = node
		if mapByPId[pId] == nil {
			mapByPId[pId] = []T{node}
		} else {
			mapByPId[pId] = append(mapByPId[pId], node)
		}
	}
	var makeChildren func(id K) []Node[T]
	makeChildren = func(id K) []Node[T] {
		elems := mapByPId[id]
		if len(elems) == 0 {
			return nil
		}
		var nodes []Node[T]
		for i := range elems {
			nodes = append(nodes, Node[T]{
				Elem: elems[i],
			})
		}
		for i := range nodes {
			nodes[i].Children = makeChildren(getId(nodes[i].Elem))
		}
		return nodes
	}
	root, ok := mapById[rootId]
	if !ok {
		return nil, errors.New("root_id not exist")
	}
	rootNode := Node[T]{
		Elem:     root,
		Children: makeChildren(rootId),
	}
	return &rootNode, nil
}

func BuildTree[T any](items []T, rootId int64, idFunc func(T) int64, parentIdFunc func(T) int64, setChildren func(T, []T) T) []T {
	itemMap := make(map[int64]Node[T])
	var roots []T

	// 构建节点缓存
	for _, item := range items {
		id := idFunc(item)
		parentId := parentIdFunc(item)

		node := Node[T]{Elem: item}
		itemMap[id] = node

		if parentId == rootId {
			roots = append(roots, item)
		} else {
			parentNode, ok := itemMap[parentId]
			if ok {
				parentNode.Children = append(parentNode.Children, node)
				itemMap[parentId] = parentNode
			} else {
				parentNode := Node[T]{Children: []Node[T]{node}}
				itemMap[parentId] = parentNode
			}
		}
	}

	// 递归构建树
	for i, root := range roots {
		node, ok := itemMap[idFunc(root)]
		if ok {
			roots[i] = convertToT(node, idFunc, setChildren)
		}
	}

	return roots
}

func convertToT[T any](node Node[T], idFunc func(T) int64, setChildren func(T, []T) T) T {
	result := node.Elem
	if len(node.Children) > 0 {
		children := make([]T, len(node.Children))
		for i := range node.Children {
			children[i] = buildSubtree(getItemMap(node.Children[i], idFunc), node.Children[i], idFunc, setChildren).Elem
		}
		result = setChildren(result, children)
	}
	return result
}

func buildSubtree[T any](itemMap map[int64]Node[T], node Node[T], idFunc func(T) int64, setChildren func(T, []T) T) Node[T] {
	children := make([]Node[T], len(node.Children))
	for i := range node.Children {
		children[i] = buildSubtree(itemMap, node.Children[i], idFunc, setChildren)
	}
	result := Node[T]{Elem: node.Elem, Children: children}
	if len(result.Children) > 0 {
		children := make([]T, len(result.Children))
		for i := range result.Children {
			children[i] = result.Children[i].Elem
		}
		result.Elem = setChildren(result.Elem, children)
	}
	return result
}

func getItemMap[T any](node Node[T], idFunc func(T) int64) map[int64]Node[T] {
	itemMap := make(map[int64]Node[T])
	getItemMapRecursively(itemMap, node, idFunc)
	return itemMap
}

func getItemMapRecursively[T any](itemMap map[int64]Node[T], node Node[T], idFunc func(T) int64) {
	itemMap[idFunc(node.Elem)] = node
	for i := range node.Children {
		getItemMapRecursively(itemMap, node.Children[i], idFunc)
	}
}

//
//
//
//func BuildTree[T any](items []T, rootId int64, idFunc func(T) int64, parentIdFunc func(T) int64, setChildren func(T, []T) T) []T {
//	itemMap := make(map[int64]Node[T])
//	var roots []T
//
//	for _, item := range items {
//		id := idFunc(item)
//		parentId := parentIdFunc(item)
//
//		node := Node[T]{Elem: item}
//		itemMap[id] = node
//
//		if parentId == rootId {
//			// 当前的节点没有父节点，所以直接放入到root中
//			roots = append(roots, item)
//		} else {
//			// 获取当前节点到父节点
//			parentNode, ok := itemMap[parentId]
//			// 如果当前节点到父节点已经在缓存列表itemMap中存在
//			if ok {
//				// 将当前节点加入到父节点到children中
//				parentNode.Children = append(parentNode.Children, node)
//				// 重新刷入到itemMap
//				itemMap[parentId] = parentNode
//			} else {
//				// 新建一个父节点
//				parentNode := Node[T]{Children: []Node[T]{node}}
//				// 重新刷入到itemMap
//				itemMap[parentId] = parentNode
//			}
//		}
//	}
//
//	// 递归处理子树
//	for i, root := range roots {
//		node, ok := itemMap[idFunc(root)]
//		if ok {
//			roots[i] = convertToT(node, idFunc, parentIdFunc, setChildren)
//		}
//	}
//
//	return roots
//}
//
//func convertToT[T any](node Node[T], idFunc func(T) int64, parentIdFunc func(T) int64, setChildren func(T, []T) T) T {
//	result := node.Elem
//
//	if len(node.Children) > 0 {
//		children := BuildTree(getElements(node.Children), idFunc(node.Elem), idFunc, parentIdFunc, setChildren)
//		result = setChildren(result, children)
//	}
//
//	return result
//}
//
//func getElements[T any](nodes []Node[T]) []T {
//	var result []T
//	for _, node := range nodes {
//		result = append(result, node.Elem)
//	}
//	return result
//}

func PrintTree[T any](nodes []T, depth int, getChildren func(T) []T) {
	for _, node := range nodes {
		for i := 0; i < depth; i++ {
			fmt.Print("  ")
		}
		fmt.Printf("%v\n", node)
		PrintTree(getChildren(node), depth+1, getChildren)
	}
}
