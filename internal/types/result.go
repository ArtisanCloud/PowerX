package types

type Page[T any] struct {
	List      []T
	PageIndex int
	PageSize  int
	Total     int64
}
