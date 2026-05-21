package pagination

type Options struct {
	first  *int
	last   *int
	before string
	after  string
}

type Option func(*Options)

func NewOptions(opts ...Option) (ret *Options) {
	ret = new(Options)
	for _, i := range opts {
		i(ret)
	}
	return
}

func WithFirst(v int) Option {
	return func(o *Options) {
		o.first = &v
	}
}

func WithLast(v int) Option {
	return func(o *Options) {
		o.last = &v
	}
}

func WithAfter(v string) Option {
	return func(o *Options) {
		o.after = v
	}
}

func WithBefore(v string) Option {
	return func(o *Options) {
		o.before = v
	}
}

func OptionFromInput(
	after *string,
	before *string,
	first *int,
	last *int,
) (s []Option) {
	if after != nil {
		s = append(s, OptionAfter(*after))
	}
	if before != nil {
		s = append(s, OptionBefore(*before))
	}
	if first != nil {
		s = append(s, OptionFirst(*first))
	}
	if last != nil {
		s = append(s, OptionLast(*last))
	}
	return s
}

func (opts Options) checkFirstLast() (err error) {
	if opts.last != nil && opts.first != nil {
		err = ErrFirstAndLastAtSameTime
		return
	}
	if opts.last == nil && opts.first == nil {
		err = ErrFirstOrLastMissing
		return
	}
	if v := opts.first; v != nil {
		err = CheckPageSize(*v)
		if err != nil {
			return
		}
	}
	if v := opts.first; v != nil {
		err = CheckPageSize(*v)
		if err != nil {
			return
		}
	}
	return
}

// Deprecated: rename to [WithFirst]
func OptionFirst(v int) Option {
	return func(o *Options) {
		o.first = &v
	}
}

// Deprecated: rename to [WithLast]
func OptionLast(v int) Option {
	return func(o *Options) {
		o.last = &v
	}
}

// Deprecated: rename to [WithAfter]
func OptionAfter(v string) Option {
	return func(o *Options) {
		o.after = v
	}
}

// Deprecated: rename to [WithBefore]
func OptionBefore(v string) Option {
	return func(o *Options) {
		o.before = v
	}
}
