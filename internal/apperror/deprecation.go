package apperror

import (
	"fmt"
	"math"
	"time"
)

func NewErrDeprecated(feature string, reason string, removalOn time.Time) *AppError {
	var removalOnText = removalOn.Format(time.RFC3339)
	return &AppError{
		Code: "DEPRECATED",
		Message: fmt.Sprintf(
			"%s has been deprecated, removal on %s",
			feature,
			removalOnText,
		),
		Locales: Locales{
			Zh: fmt.Sprintf(
				"%s 已废弃，%s 移除",
				feature,
				removalOnText,
			),
		},
		Extensions: map[string]interface{}{
			"reason": reason,
		},
	}
}

func NewErrBrownoutDeprecated(feature string, reason string, removalOn, endTime, nextStartTime time.Time) *AppError {
	var now = time.Now()
	var remainsMinute = math.Ceil(endTime.Sub(now).Minutes())
	var durationMinute = math.Ceil(nextStartTime.Sub(endTime).Minutes())
	return &AppError{
		Code: "DEPRECATED",
		Message: fmt.Sprintf(
			"%s has been deprecated and is in a brownout state, will be available for %.0f minute(s) after %.0f minute(s).",
			feature,
			durationMinute,
			remainsMinute,
		),
		Locales: Locales{
			Zh: fmt.Sprintf(
				"%s 已废弃并正处于限制状态，将在 %.0f 分钟后可用 %.0f 分钟。",
				feature,
				remainsMinute,
				durationMinute,
			),
		},
		Extensions: map[string]interface{}{
			"reason": reason,
		},
	}
}

var DeprecationDelay time.Duration

type BrownoutResult struct {
	StartTime     time.Time
	EndTime       time.Time
	NextStartTime time.Time
}

type BrownoutRule = func(removalOn, now time.Time) (res BrownoutResult)
type BrownoutOptions struct {
	rule BrownoutRule
	now  time.Time
}

type BrownoutOption = func(opts *BrownoutOptions)

func BrownoutOptionRule(rule func(removalOn, now time.Time) BrownoutResult) BrownoutOption {
	return func(opts *BrownoutOptions) {
		opts.rule = rule
	}
}
func BrownoutOptionNow(v time.Time) BrownoutOption {
	return func(opts *BrownoutOptions) {
		opts.now = v
	}
}

var DefaultBrownoutRule BrownoutRule = func(removalOn, now time.Time) (res BrownoutResult) {
	var remains = removalOn.Sub(now)
	var m = now.Minute()

	if remains < 3*24*time.Hour {
		if m < 20 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 20, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 30, 0, 0, now.Location())
			return
		}
		if m >= 30 && m < 50 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 30, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 50, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
			return
		}
	} else if remains < 7*24*time.Hour {
		if m < 15 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 15, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 30, 0, 0, now.Location())
		} else if m >= 30 && m < 45 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 30, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 45, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
		}
	} else if remains < 2*7*24*time.Hour {
		if m < 15 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 15, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 30, 0, 0, now.Location())
		} else if m >= 30 && m < 40 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 30, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 40, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
		}
	} else if remains < 3*7*24*time.Hour {
		if m < 15 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 15, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
		}
	} else if remains < 4*7*24*time.Hour {
		if m < 10 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 10, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
		}
	} else if remains < 6*7*24*time.Hour {
		if m < 1 {
			res.StartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
			res.EndTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 1, 0, 0, now.Location())
			res.NextStartTime = time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
		}
	}
	return
}

func Brownout(feature, reason string, removalOn time.Time, options ...BrownoutOption) error {
	var opts = new(BrownoutOptions)
	opts.now = time.Now()
	opts.rule = DefaultBrownoutRule
	for _, i := range options {
		i(opts)
	}

	removalOn = removalOn.Add(DeprecationDelay)

	if opts.now.After(removalOn) {
		return NewErrDeprecated(feature, reason, removalOn)
	}

	var res = opts.rule(removalOn, opts.now)
	if res.StartTime.IsZero() {
		return nil
	}

	return NewErrBrownoutDeprecated(
		feature,
		reason,
		removalOn,
		res.EndTime,
		res.NextStartTime,
	)
}
