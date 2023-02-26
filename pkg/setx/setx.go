package setx

type Set[T comparable] interface {
	Add(...T) Set[T]
	Remove(values ...T) Set[T]
	Contains(values ...T) bool
	Diff(set hashSet[T]) Set[T]
	Slice() []T
	String() string
	Length() int
}
