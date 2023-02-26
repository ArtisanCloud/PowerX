package mapx

func Keys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func Values[K comparable, V any](m map[K]V) []V {
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

func HasKeys[K comparable, V any](m map[K]V, ks ...K) bool {
	for _, k := range ks {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

func MapByFunc[T any, K comparable, V any](s []T, fun func(item T) (K, V)) map[K]V {
	m := make(map[K]V, len(s))
	for i := range s {
		k, v := fun(s[i])
		m[k] = v
	}
	return m
}
