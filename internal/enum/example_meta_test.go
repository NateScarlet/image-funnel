// enum package provide a way to define enum type
package enum_test

import (
	"fmt"
	"main/internal/enum"
)

type Session2 struct {
	index int
}

var session2 = enum.New[Session2]()
var ParseSession2 = session2.Parse
var (
	SSpring2 = session2.Define("SPRING", Session2{1})
	SSummer2 = session2.Define("SUMMER", Session2{2})
	SAutumn2 = session2.Define("AUTUMN", Session2{3})
	SWinter2 = session2.Define("WINTER", Session2{4})
)

func Example_meta() {
	fmt.Println(SSpring2.Meta().index)
	var v, _ = ParseSession2("SPRING")
	fmt.Println(v == SSpring2)
	fmt.Println(v.Meta().index)
	fmt.Println(enum.Enum[Session2]{}.Meta().index)
	// Output:
	// 1
	// true
	// 1
	// 0
}
