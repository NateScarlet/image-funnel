package pagination

import "encoding/hex"

// TextToCursor 把文本键编码为不透明游标。
//
// 采用十六进制编码：与原文保持相同的字典序（因此跨页可直接比较），
// 同时对前端不透明，调用方不应依赖具体编码。
func TextToCursor(v string) string {
	return hex.EncodeToString([]byte(v))
}

// Deprecated: use `ByTextV2` instead
func ByText(options ...Option) (
	newInfo func(
		startCursor, endCursor string,
		hasPreviousPage, hasNextPage bool,
	) *PageInfo,
	after, before string,
	limit int,
	reverse bool,
	err error,
) {
	var opts = NewOptions(options...)
	err = opts.checkFirstLast()
	if err != nil {
		return
	}
	before = opts.before
	after = opts.after
	if opts.last != nil {
		limit = *opts.last
		reverse = true
		after, before = before, after
	}
	if opts.first != nil {
		limit = *opts.first
	}
	newInfo = func(
		start, end string,
		hasPreviousPage, hasNextPage bool,
	) *PageInfo {
		return &PageInfo{
			HasPreviousPage: hasPreviousPage,
			HasNextPage:     hasNextPage,
			StartCursor:     start,
			EndCursor:       end,
		}
	}
	return
}

func ByTextV2[T any](
	seq func(func(T, error) bool),
	cursor func(T) (string, error),
	w Writer[T],
	options ...Option,
) error {
	var opts = NewOptions(options...)
	var err = opts.checkFirstLast()
	if err != nil {
		return err
	}
	type item struct {
		index  int
		value  T
		cursor string
	}
	var match = func(yield func(*item, error) bool) {
		var nextIndex int
		for i, err := range seq {
			if err != nil {
				if yield(nil, err) {
					continue
				}
				return
			}
			c, err := cursor(i)
			if err != nil {
				if yield(nil, err) {
					continue
				}
				return
			}
			if !yield(&item{nextIndex, i, c}, err) {
				return
			}
			nextIndex++
		}
	}
	if before := opts.before; before != "" {
		var seq = match
		match = func(yield func(*item, error) bool) {
			for i, err := range seq {
				if err != nil {
					if yield(i, err) {
						continue
					}
					return
				}
				// 游标有序：遇到不小于 before 的项后，后面都不属于本页
				if before <= i.cursor {
					w.WriteHasNextPage(true)
					return
				}
				if !yield(i, err) {
					return
				}
			}
		}
	}
	if after := opts.after; after != "" {
		var seq = match
		match = func(yield func(*item, error) bool) {
			for i, err := range seq {
				if err != nil {
					if yield(i, err) {
						continue
					}
					return
				}
				// 跳过游标自身及之前的所有项，只返回严格位于其后的项
				if i.cursor <= after {
					w.WriteHasPreviousPage(true)
					continue
				}
				if !yield(i, err) {
					return
				}
			}
		}
	}
	if v := opts.first; v != nil {
		var seq = match
		match = func(yield func(*item, error) bool) {
			var n int
			for i, err := range seq {
				if err != nil {
					if yield(i, err) {
						continue
					}
					return
				}
				if n == *v {
					w.WriteHasNextPage(true)
					return
				}
				if !yield(i, err) {
					return
				}
				n++
			}
		}
	}
	if v := opts.last; v != nil {
		var seq = match
		match = func(yield func(*item, error) bool) {
			var items []*item
			for i, err := range seq {
				if err != nil {
					if yield(nil, err) {
						continue
					}
					return
				}
				items = append(items, i)
			}
			if len(items) > *v {
				items = items[len(items)-*v:]
				w.WriteHasPreviousPage(true)
			}
			for _, i := range items {
				if !yield(i, nil) {
					return
				}
			}
		}
	}
	for i, err := range match {
		if err != nil {
			return err
		}
		err = w.Write(i.value, i.cursor)
		if err != nil {
			return err
		}
	}
	return nil
}
