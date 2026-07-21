package pagination

import "main/internal/shared"

// PageInfo 是 shared.PageInfoDTO 的类型别名
type PageInfo = shared.PageInfoDTO

// ReversePageInfo 反转分页信息的方向
func ReversePageInfo(pi *PageInfo) {
	pi.HasNextPage, pi.HasPreviousPage = pi.HasPreviousPage, pi.HasNextPage
	pi.StartCursor, pi.EndCursor = pi.EndCursor, pi.StartCursor
}

// UpdatePageInfoCursor 更新分页信息的光标
func UpdatePageInfoCursor(pi *PageInfo, cursor string) {
	if pi.StartCursor == "" {
		pi.StartCursor = cursor
	}
	pi.EndCursor = cursor
}