package utils

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Max1(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func MaxInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
