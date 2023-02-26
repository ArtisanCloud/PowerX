package treex

import "errors"

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
