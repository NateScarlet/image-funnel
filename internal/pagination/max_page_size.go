package pagination

import "fmt"

var MaxPageSize int = 1000

func CheckPageSize(size int) error {
	if size < 0 {
		return fmt.Errorf("page size must be positive")
	}
	if MaxPageSize > 0 && size > MaxPageSize {
		return fmt.Errorf("select match %d items, max page size is %d", size, MaxPageSize)
	}
	return nil
}
