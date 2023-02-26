package setx

import (
	"fmt"
	"strings"
)

type hashSet[T comparable] struct {
	hashMap map[T]struct{}
}

func NewHashSet[T comparable](values ...T) Set[T] {
	hashMap := make(map[T]struct{})
	for _, value := range values {
		hashMap[value] = struct{}{}
	}
	return &hashSet[T]{
		hashMap: hashMap,
	}
}

func (s *hashSet[T]) Add(values ...T) Set[T] {
	for _, value := range values {
		s.hashMap[value] = struct{}{}
	}
	return s
}

func (s *hashSet[T]) Remove(values ...T) Set[T] {
	for _, value := range values {
		delete(s.hashMap, value)
	}
	return s
}

func (s *hashSet[T]) Contains(values ...T) bool {
	for _, value := range values {
		if _, ok := s.hashMap[value]; !ok {
			return false
		}
	}
	return true
}

func (s *hashSet[T]) Diff(set hashSet[T]) Set[T] {
	for key := range set.hashMap {
		delete(s.hashMap, key)
	}
	return s
}

func (s *hashSet[T]) Slice() []T {
	slice := make([]T, 0, len(s.hashMap))
	for key := range s.hashMap {
		slice = append(slice, key)
	}
	return slice
}

func (s *hashSet[T]) String() string {
	var builder strings.Builder
	for key := range s.hashMap {
		builder.WriteString(fmt.Sprintf("%v", key))
	}
	return builder.String()
}

func (s *hashSet[T]) Length() int {
	return len(s.hashMap)
}
