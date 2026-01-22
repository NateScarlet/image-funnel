package apperror

import (
	"context"
	"errors"
)

// As try convert any error to target, return true if converted.
func As(err error, target **AppError) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		*target = ErrTimeout
		return true
	}
	return errors.As(err, target)
}
