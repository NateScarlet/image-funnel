package pagination

type PageInfo struct {
	// When paginating forwards, are there more items?
	HasNextPage bool `json:"hasNextPage"`
	// When paginating backwards, are there more items?
	HasPreviousPage bool `json:"hasPreviousPage"`
	// When paginating backwards, the cursor to continue.
	StartCursor *string `json:"startCursor"`
	// When paginating forwards, the cursor to continue.
	EndCursor *string `json:"endCursor"`
}

func (pi *PageInfo) Reverse() {
	pi.HasNextPage, pi.HasPreviousPage = pi.HasPreviousPage, pi.HasNextPage
	pi.StartCursor, pi.EndCursor = pi.EndCursor, pi.StartCursor
}

func (pi *PageInfo) UpdateCursor(v string) {
	if pi.StartCursor == nil {
		pi.StartCursor = &v
	}
	pi.EndCursor = &v
}

func NewPageInfo(
	startCursor, endCursor string,
	hasPreviousPage, hasNextPage bool,
) *PageInfo {
	var v = new(PageInfo)
	v.HasPreviousPage = hasPreviousPage
	v.HasNextPage = hasNextPage
	if startCursor != "" {
		v.StartCursor = &startCursor
	}
	if endCursor != "" {
		v.EndCursor = &endCursor
	}
	return v
}
