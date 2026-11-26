package sentry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"main/internal/apperror"

	"github.com/stretchr/testify/assert"
)

func TestShouldReport(t *testing.T) {
	known := apperror.New("BAD_REQUEST", "bad request", "请求错误")
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"nil":              {nil, false},
		"unexpected":       {errors.New("disk full"), true},
		"known user error": {known, false},
		"wrapped known":    {fmt.Errorf("handle request: %w", known), false},
		"canceled":         {context.Canceled, false},
		"deadline":         {context.DeadlineExceeded, false},
		"not found":        {os.ErrNotExist, false},
		"end of stream":    {io.EOF, false},
		"empty query":      {sql.ErrNoRows, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, ShouldReport(tc.err))
		})
	}
}

func TestNoopReporterIsSafeZeroValue(t *testing.T) {
	var reporter Reporter = NoopReporter
	assert.NoError(t, reporter.Report(context.Background(), Event{Err: errors.New("boom")}))
}
