package pagination

import "main/internal/apperror"

func ByDirection(options []Option, reverse bool) (direction OrderDirection, after string, limit int, err error) {
	var opts = NewOptions(options...)
	if opts.last == nil && opts.first == nil {
		err = apperror.New(
			"INVALID_INPUT",
			"pagination is required (use `first` or `last`)",
			"必须分页（使用 `first` 或 `last`)",
		)
		return
	} else if opts.first != nil {
		limit = *opts.first
		after = opts.after
		if opts.before != "" {
			err = apperror.New(
				"INVALID_INPUT",
				"`before` is not supported during forward pagination",
				"向后分页时不支持 `before` 参数",
			)
			return
		}
	} else {
		limit = *opts.last
		after = opts.before
		if opts.after != "" {
			err = apperror.New(
				"INVALID_INPUT",
				"`after` is not supported during backward pagination",
				"向前分页时不支持 `after` 参数",
			)
			return
		}
		reverse = !reverse
	}
	if reverse {
		direction = ODDescend
	} else {
		direction = ODAscend
	}
	err = CheckPageSize(limit)
	if err != nil {
		return
	}
	return
}
