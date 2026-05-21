package pagination

func ByIndex[T any](seq func(func(T) bool), w Writer[T], options ...Option) error {
	var opts = NewOptions(options...)
	var err = opts.checkFirstLast()
	if err != nil {
		return err
	}
	var match = func(yield func(int, T) bool) {
		var nextIndex int
		for i := range seq {
			if !yield(nextIndex, i) {
				return
			}
			nextIndex++
		}
	}
	if v := opts.before; v != "" {
		var seq = match
		before, err := IndexFromCursor(v)
		if err != nil {
			return err
		}
		match = func(yield func(int, T) bool) {
			for index, i := range seq {
				if index >= before {
					w.WriteHasNextPage(true)
					return
				}
				if !yield(index, i) {
					return
				}
			}
		}
	}
	if v := opts.after; v != "" {
		after, err := IndexFromCursor(opts.after)
		if err != nil {
			return err
		}
		var seq = match
		match = func(yield func(int, T) bool) {
			for index, i := range seq {
				if index <= after {
					w.WriteHasPreviousPage(true)
					continue
				}
				if !yield(index, i) {
					return
				}
			}
		}
	}
	if v := opts.first; v != nil {
		var seq = match
		match = func(yield func(int, T) bool) {
			var n int
			for index, i := range seq {
				if n == *v {
					w.WriteHasNextPage(true)
					return
				}
				if !yield(index, i) {
					return
				}
				n++
			}
		}
	}
	if v := opts.last; v != nil {
		var seq = match
		match = func(yield func(int, T) bool) {
			type item struct {
				index int
				value T
			}
			var items []item
			for index, i := range seq {
				items = append(items, item{index, i})
			}
			if len(items) > *v {
				items = items[len(items)-*v:]
				w.WriteHasPreviousPage(true)
			}
			for _, i := range items {
				if !yield(i.index, i.value) {
					return
				}
			}
		}
	}
	for index, i := range match {
		err = w.Write(i, IndexToCursor(index))
		if err != nil {
			return err
		}
	}
	return nil
}

func ByIndexE[T any](seq func(func(T, error) bool), w Writer[T], options ...Option) (err error) {
	var iterErr error
	err = ByIndex(func(yield func(T) bool) {
		for i, err := range seq {
			if err != nil {
				iterErr = err
				return
			}
			if !yield(i) {
				return
			}
		}
	}, w, options...)
	if iterErr != nil {
		err = iterErr
	}
	return err
}
