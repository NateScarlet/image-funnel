package enum_test

import (
	"fmt"
	"main/internal/enum"
	"slices"
)

type SessionMeta struct{}

type Session = enum.Enum[SessionMeta]

// enum class should not expose to other module
var session = enum.New[SessionMeta]()

// panic on create multiple class for same meta type
// var _ = enum.New[Session]()

var (
	SSpring      = session.Define("SPRING")
	SSummer      = session.Define("SUMMER")
	SAutumn      = session.Define("AUTUMN")
	SWinter      = session.Define("WINTER")
	SWinterAlias = session.DefineAlias("WINTER_ALIAS", "WINTER")
)

type OtherEnumMeta struct{}

type OtherEnum = enum.Enum[OtherEnumMeta]

var otherEnum = enum.New[OtherEnumMeta]()
var (
	OE1 = otherEnum.Define("1")
)

func Example_basic() {
	// only defined enum or zero value is assignable
	var v Session = SSummer
	// other enum is not assignable
	// v = OE1
	v.UnmarshalJSON([]byte("\"WINTER\""))
	fmt.Println(v)
	fmt.Println(SSpring, SSummer, SAutumn, SWinter)
	fmt.Println(SWinterAlias)
	fmt.Println(len(slices.Collect(session.Values())))
	v, _ = enum.Parse[SessionMeta]("SPRING")
	fmt.Println(v == SSpring)
	v, _ = enum.Parse[SessionMeta]("WINTER_ALIAS")
	fmt.Println(v == SWinter)
	// Output:
	// WINTER
	// SPRING SUMMER AUTUMN WINTER
	// WINTER
	// 4
	// true
	// true
}
