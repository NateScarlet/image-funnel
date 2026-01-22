package apperror

import (
	"errors"
	"iter"
)

type joinError interface {
	Unwrap() []error
}

func ExpandJoinError(err error) iter.Seq[error] {
	return func(yield func(error) bool) {
		expandJoinError(yield, err)
	}
}

func expandJoinError(yield func(error) bool, err error) bool {
	if err == nil {
		return true
	}
	if errs := (joinError)(nil); errors.As(err, &errs) {
		_, exact := err.(joinError)
		if !exact && !yield(err) {
			return false
		}
		for _, err := range errs.Unwrap() {
			if !expandJoinError(yield, err) {
				return false
			}
		}
		return true
	} else {
		return yield(err)
	}
}
