package pagination

import (
	"fmt"
	"strings"
)

const base62Chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func IndexToCursor(index int) string {
	if index == 0 {
		return string(base62Chars[0])
	}

	var result []byte
	for index > 0 {
		remainder := index % 62
		result = append(result, base62Chars[remainder])
		index /= 62
	}

	// 反转结果
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

func IndexFromCursor(v string) (int, error) {
	var result int
	for _, char := range v {
		index := strings.IndexRune(base62Chars, char)
		if index == -1 {
			return 0, fmt.Errorf("invalid base62 character: %c", char)
		}
		result = result*62 + index
	}
	return result, nil
}
