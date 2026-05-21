package pagination

import (
	"main/internal/enum"
)

type OrderDirectionMeta struct {
	num int
}

var orderDirection = enum.New[OrderDirectionMeta]()

type OrderDirection = enum.Enum[OrderDirectionMeta]

var (
	ODAscend  = orderDirection.Define("ASC", OrderDirectionMeta{1})
	ODDescend = orderDirection.Define("DESC", OrderDirectionMeta{-1})
)

func (obj OrderDirectionMeta) Number() int {
	return obj.num
}
func (obj OrderDirectionMeta) Reverse() OrderDirection {
	if obj.num == 1 {
		return ODDescend
	}
	return ODAscend
}
