package mathx

import "math/rand"

// 生成指定范围内指定数量的不重复随机整数
func GenerateRandomNumbers(count, min, max int) []int {
	numbers := make([]int, 0, count)
	uniqueNumbers := make(map[int]struct{})

	for len(numbers) < count {
		randomNumber := rand.Intn(max-min+1) + min
		if _, exists := uniqueNumbers[randomNumber]; !exists {
			numbers = append(numbers, randomNumber)
			uniqueNumbers[randomNumber] = struct{}{}
		}
	}

	return numbers
}
