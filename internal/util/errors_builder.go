package util

import (
	"errors"
	"fmt"
)

func NewErrorsBuilder(cap int) *ErrorsBuilder {
	return &ErrorsBuilder{
		nil,
		0,
		cap,
	}
}

// ErrorsBuilder collect multiple error.
//
// use [NewErrorsBuilder] to specify cap, otherwise 16 is default.
type ErrorsBuilder struct {
	errs       []error
	errorCount int
	cap        int
}

func (b *ErrorsBuilder) Len() int {
	return len(b.errs)
}

func (b *ErrorsBuilder) Cap() int {
	if b.cap == 0 {
		return 16
	}
	return b.cap
}

func (b *ErrorsBuilder) Add(err error) {
	if err == nil {
		return
	}
	if b.errs == nil {
		b.errs = make([]error, 0, b.Cap())
	}
	if len(b.errs) < cap(b.errs) {
		b.errs = append(b.errs, err)
	}
	b.errorCount++
}

func (b ErrorsBuilder) Build() error {
	if b.errorCount == 0 {
		return nil
	}
	if b.errorCount == 1 {
		return b.errs[0]
	}
	if b.errorCount == len(b.errs) {
		return errors.Join(b.errs...)
	}
	return fmt.Errorf("%w\n...(%d more errors)", errors.Join(b.errs...), b.errorCount-len(b.errs))
}
