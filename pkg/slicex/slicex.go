package slicex

func SlicePluck[T any, V comparable](s []T, getValue func(item T) V) []V {
	var ks []V
	for _, t := range s {
		ks = append(ks, getValue(t))
	}
	return ks
}

func Contains[T comparable](slice []T, values ...T) bool {
	var set map[T]struct{}
	set = make(map[T]struct{}, 0)
	for _, i := range slice {
		set[i] = struct{}{}
	}
	for _, value := range values {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func Filter[T any](slice []T, fun func(item T) bool) []T {
	var s []T
	for _, item := range slice {
		if fun(item) {
			s = append(s, item)
		}
	}
	return s
}

func Concatenate[T any](s []T, objs ...[]T) []T {
	for i := 0; i < len(objs); i++ {
		for j := 0; j < len(objs[i]); j++ {
			s = append(s, objs[i][j])
		}
	}
	return s
}
