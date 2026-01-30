package scalar

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/NateScarlet/iso8601/pkg/iso8601"
)

// Duration that use iso8601 format and
// implements bson (truncate to milliseconds),
// and graphql interfaces.
type Duration struct {
	raw  string
	nano float64
	v    iso8601.Duration
}

type DurationOptions struct {
	hours        int64
	minutes      int64
	seconds      int64
	milliseconds int64
	microseconds int64
	nanoseconds  int64
}

func newDurationOptions(options ...DurationOption) *DurationOptions {
	var opts = new(DurationOptions)
	for _, i := range options {
		i(opts)
	}
	return opts
}

type DurationOption func(opts *DurationOptions)

func DurationWithHours(v int64) DurationOption {
	return func(opts *DurationOptions) {
		opts.hours = v
	}
}

func DurationWithMinutes(v int64) DurationOption {
	return func(opts *DurationOptions) {
		opts.minutes = v
	}
}

func DurationWithSeconds(v int64) DurationOption {
	return func(opts *DurationOptions) {
		opts.seconds = v
	}
}

func DurationWithMilliseconds(v int64) DurationOption {
	return func(opts *DurationOptions) {
		opts.milliseconds = v
	}
}

func DurationWithMicroseconds(v int64) DurationOption {
	return func(opts *DurationOptions) {
		opts.microseconds = v
	}
}

func DurationWithNanoseconds(v int64) DurationOption {
	return func(opts *DurationOptions) {
		opts.nanoseconds = v
	}
}

func addToDuration(d *iso8601.Duration, v int64, unit time.Duration) {
	if v == 0 {
		return
	}
	if (v < 0 && *d == iso8601.Duration{}) {
		d.Negative = true
	}
	if d.Negative {
		v = -v
		if v < 0 {
			// carrying
			switch unit {
			case time.Hour:
			case time.Minute:
				d.Hours += v / 60
				v %= 60
				if v < 0 {
					d.Hours--
					v += 60
				}
			case time.Second:
				d.Minutes += v / 60
				v %= 60
				if v < 0 {
					d.Minutes--
					v += 60
				}
			case time.Millisecond:
				d.Seconds += v / 1e3
				v %= 1e3
				if v < 0 {
					d.Seconds--
					v += 1e3
				}
			case time.Microsecond:
				d.Seconds += v / 1e6
				v %= 1e6
				if v < 0 {
					d.Seconds--
					v += 1e6
				}
			case time.Nanosecond:
				d.Seconds += v / 1e9
				v %= 1e9
				if v < 0 {
					d.Seconds--
					v += 1e9
				}
			default:
				panic("unexpected unit")
			}
		}
	}
	switch unit {
	case time.Hour:
		d.Hours += v
	case time.Minute:
		var m = v
		d.Hours += m / int64(time.Hour/time.Minute)
		m %= int64(time.Hour / time.Minute)
		d.Minutes += m
	case time.Second:
		var s = v
		d.Hours += s / int64(time.Hour/time.Second)
		s %= int64(time.Hour / time.Second)
		d.Minutes += s / int64(time.Minute/time.Second)
		s %= int64(time.Minute / time.Second)
		d.Seconds += s
	case time.Millisecond:
		var ms = v
		d.Hours += ms / int64(time.Hour/time.Millisecond)
		ms %= int64(time.Hour / time.Millisecond)
		d.Minutes += ms / int64(time.Minute/time.Millisecond)
		ms %= int64(time.Minute / time.Millisecond)
		d.Seconds += ms / int64(time.Second/time.Millisecond)
		ms %= int64(time.Second / time.Millisecond)
		d.Nanoseconds += ms * int64(time.Millisecond)
	case time.Microsecond:
		var us = v
		d.Hours += us / int64(time.Hour/time.Microsecond)
		us %= int64(time.Hour / time.Microsecond)
		d.Minutes += us / int64(time.Minute/time.Microsecond)
		us %= int64(time.Minute / time.Microsecond)
		d.Seconds += us / int64(time.Second/time.Microsecond)
		us %= int64(time.Second / time.Microsecond)
		d.Nanoseconds += us * int64(time.Microsecond)
	case time.Nanosecond:
		var ns = v
		d.Hours += ns / int64(time.Hour)
		ns %= int64(time.Hour)
		d.Minutes += ns / int64(time.Minute)
		ns %= int64(time.Minute)
		d.Seconds += ns / int64(time.Second)
		ns %= int64(time.Second)
		d.Nanoseconds += ns
	default:
		panic("unexpected unit")
	}

	// normalize
	if d.Negative && d.Hours <= 0 && d.Minutes <= 0 && d.Seconds <= 0 && d.Nanoseconds <= 0 {
		d.Negative = false
		d.Hours = -d.Hours
		d.Minutes = -d.Minutes
		d.Seconds = -d.Seconds
		d.Nanoseconds = -d.Nanoseconds
	}
	d.Seconds += d.Nanoseconds / int64(time.Second)
	d.Nanoseconds %= int64(time.Second)

	d.Minutes += d.Seconds / 60
	d.Seconds %= 60

	d.Hours += d.Minutes / 60
	d.Minutes %= 60
}

func durationNano(v iso8601.Duration) (ret float64) {
	ret += float64(v.Years) * float64(iso8601.Year)
	ret += float64(v.Months) * float64(iso8601.Month)
	ret += float64(v.Weeks) * float64(iso8601.Week)
	ret += float64(v.Days) * float64(iso8601.Day)
	ret += float64(v.Hours) * float64(time.Hour)
	ret += float64(v.Minutes) * float64(time.Minute)
	ret += float64(v.Seconds) * float64(time.Second)
	ret += float64(v.Nanoseconds)
	if v.Negative {
		ret *= -1
	}
	return
}

func NewDuration(options ...DurationOption) Duration {
	var opts = newDurationOptions(options...)
	d := new(iso8601.Duration)
	addToDuration(d, opts.hours, time.Hour)
	addToDuration(d, opts.minutes, time.Minute)
	addToDuration(d, opts.seconds, time.Second)
	addToDuration(d, opts.milliseconds, time.Millisecond)
	addToDuration(d, opts.microseconds, time.Microsecond)
	addToDuration(d, opts.nanoseconds, time.Nanosecond)
	if d.Negative && (d.Hours < 0 || d.Minutes < 0 || d.Seconds < 0 || d.Nanoseconds < 0) {
		panic(iso8601.ErrOverflow)
	}
	return Duration{d.String(), durationNano(*d), *d}
}

func DurationFromStandard(t time.Duration) Duration {
	return NewDuration(DurationWithNanoseconds(int64(t)))
}

func addToDurationFloat64(d *iso8601.Duration, nano float64, unit time.Duration) float64 {
	var n = int64(math.Trunc(nano / float64(unit)))
	if n == 0 {
		return nano
	}
	addToDuration(d, n, unit)
	return math.Mod(nano, float64(unit))
}

func DurationFromFloat64Nano(v float64) Duration {
	d := new(iso8601.Duration)
	var nano = v
	nano = addToDurationFloat64(d, nano, time.Hour)
	nano = addToDurationFloat64(d, nano, time.Minute)
	nano = addToDurationFloat64(d, nano, time.Second)
	nano = addToDurationFloat64(d, nano, time.Millisecond)
	nano = addToDurationFloat64(d, nano, time.Nanosecond)
	return Duration{d.String(), v, *d}
}

// ParseDuration iso8601 duration string.
func ParseDuration(s string) (ret Duration, err error) {
	d, err := iso8601.ParseDuration(s)
	if err != nil {
		return
	}
	ret = Duration{s, durationNano(d), d}
	return
}

// MustParseDuration from string, panic when error.
func MustParseDuration(s string) Duration {
	dd, err := ParseDuration(s)
	if err != nil {
		panic(err)
	}
	return dd
}

func (d Duration) IsZero() bool {
	return d.raw == "" || d.raw == "P0D"
}

// String returns iso8601 duration string.
func (d Duration) String() string {
	if d.raw == "" {
		return "P0D"
	}
	return d.raw
}

// MarshalJSON implements the json.Marshaler interface
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

var _ json.Marshaler = Duration{}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(data []byte) (err error) {
	var s = new(string)
	err = json.Unmarshal(data, s)
	if err != nil {
		return
	}
	*d, err = ParseDuration(*s)
	return
}

var _ json.Unmarshaler = (*Duration)(nil)

// MarshalGQL implements the graphql.Marshaler interface
func (d Duration) MarshalGQL(w io.Writer) {
	w.Write([]byte(`"` + d.String() + `"`))
}

var _ graphql.Marshaler = Duration{}

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (d *Duration) UnmarshalGQL(value interface{}) (err error) {
	switch v := value.(type) {
	case string:
		*d, err = ParseDuration(v)
		return
	case int64:
		*d = NewDuration(DurationWithMilliseconds(v))
		return
	case float64:
		*d = NewDuration(DurationWithMilliseconds(int64(v)))
		return
	default:
		err = errors.New("duration must be string or number")
		return
	}
}

var _ graphql.Unmarshaler = (*Duration)(nil)

func (obj Duration) Nanoseconds() (ret float64) {
	return obj.nano
}

func (obj Duration) Microseconds() float64 {
	return obj.Nanoseconds() / float64(time.Microsecond)
}

func (obj Duration) Milliseconds() float64 {
	return obj.Nanoseconds() / float64(time.Millisecond)
}

func (obj Duration) Seconds() float64 {
	return obj.Nanoseconds() / float64(time.Second)
}

func (obj Duration) Minutes() float64 {
	return obj.Nanoseconds() / float64(time.Minute)
}

func (obj Duration) Hours() float64 {
	return obj.Nanoseconds() / float64(time.Hour)
}

func (obj Duration) Standard() (_ time.Duration, err error) {
	return obj.v.TimeDuration()
}

func (obj Duration) MustStandard() time.Duration {
	var v, err = obj.v.TimeDuration()
	if err != nil {
		panic(err)
	}
	return v
}

// Trunc returns the result of rounding d toward zero to a multiple of unit.
func (d Duration) Trunc(unit Duration) (_ Duration) {
	var v = d.Nanoseconds() - math.Mod(d.Nanoseconds(), unit.Nanoseconds())
	return DurationFromFloat64Nano(v)
}

// Ceil returns the least multiple of unit that greater than d.
func (d Duration) Ceil(unit Duration) (_ Duration) {
	var v = d.Nanoseconds() - math.Mod(d.Nanoseconds(), unit.Nanoseconds())
	if v < d.Nanoseconds() {
		v += unit.Nanoseconds()
	}
	return DurationFromFloat64Nano(v)
}

// Floor returns the greatest multiple of unit that less than d.
func (d Duration) Floor(unit Duration) (_ Duration) {
	var v = d.Nanoseconds() - math.Mod(d.Nanoseconds(), unit.Nanoseconds())
	if v > d.Nanoseconds() {
		v -= unit.Nanoseconds()
	}
	return DurationFromFloat64Nano(v)
}

func (obj Duration) Multiply(x float64) Duration {
	return DurationFromFloat64Nano(obj.Nanoseconds() * x)
}

func (obj Duration) Add(x Duration) Duration {
	return DurationFromFloat64Nano(obj.Nanoseconds() + x.Nanoseconds())
}

func (obj Duration) Sub(x Duration) Duration {
	return DurationFromFloat64Nano(obj.Nanoseconds() - x.Nanoseconds())
}

func (obj Duration) Abs() Duration {
	if obj.Nanoseconds() > 0 {
		return obj
	}
	return obj.Multiply(-1)
}

// Deprecated: use [util.UnwrapPointer] instead.
func IndirectDuration(v *Duration) Duration {
	if v == nil {
		return Duration{}
	}
	return *v
}
