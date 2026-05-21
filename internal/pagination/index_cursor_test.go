package pagination

import "fmt"

func ExampleIndexToCursor() {
	numbers := []int{1, 255, 1000, 123456789, 9223372036854775807}
	for _, n := range numbers {
		encoded := IndexToCursor(n)
		decoded, err := IndexFromCursor(encoded)
		if err != nil {
			fmt.Printf("Error decoding %s: %v\n", encoded, err)
			continue
		}
		fmt.Printf("%d -> %s -> %d\n", n, encoded, decoded)
	}
	// Output:
	// 1 -> 1 -> 1
	// 255 -> 47 -> 255
	// 1000 -> g8 -> 1000
	// 123456789 -> 8m0Kx -> 123456789
	// 9223372036854775807 -> aZl8N0y58M7 -> 9223372036854775807
}
