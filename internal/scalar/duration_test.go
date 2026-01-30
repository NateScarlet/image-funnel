package scalar

import (
	"math"
	"testing"
	"time"

	"github.com/NateScarlet/iso8601/pkg/iso8601"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDuration(t *testing.T) {
	for _, c := range []struct {
		name string
		give []DurationOption
		want string
	}{
		{name: "empty", give: nil, want: "P0D"},
		{
			name: "should supports negative milliseconds",
			give: []DurationOption{DurationWithMilliseconds(-1234567890)},
			want: "-PT342H56M7.89S",
		},
		{
			name: "should supports max int64 milliseconds",
			give: []DurationOption{DurationWithMilliseconds(math.MaxInt64)},
			want: "PT2562047788015H12M55.807S",
		},
		{
			name: "should supports min int64 + 1 milliseconds",
			give: []DurationOption{DurationWithMilliseconds(math.MinInt64 + 1)},
			want: "-PT2562047788015H12M55.807S",
		},
		{
			name: "should supports max int64 hours",
			give: []DurationOption{DurationWithHours(math.MaxInt64)},
			want: "PT9223372036854775807H",
		},
		{
			name: "should supports min int64 + 1 hours",
			give: []DurationOption{DurationWithHours(math.MinInt64 + 1)},
			want: "-PT9223372036854775807H",
		},
		{
			name: "should supports negative hours and minutes",
			give: []DurationOption{DurationWithHours(-1), DurationWithMinutes(-1)},
			want: "-PT1H1M",
		},
		{
			name: "should supports negative hours and positive minutes",
			give: []DurationOption{DurationWithHours(-1), DurationWithMinutes(1)},
			want: "-PT59M",
		},
		{
			name: "should supports mixed carrying",
			give: []DurationOption{DurationWithHours(-1), DurationWithMinutes(120)},
			want: "PT1H",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := NewDuration(c.give...)
			assert.Equal(t, c.want, got.String())
		})
	}
}

func TestDurationString(t *testing.T) {
	for _, c := range []struct {
		duration Duration
		expected string
	}{
		{duration: Duration{}, expected: "P0D"},
		{duration: DurationFromStandard(time.Hour), expected: "PT1H"},
		{duration: DurationFromStandard(24 * time.Hour), expected: "PT24H"},
		{duration: DurationFromStandard(time.Minute), expected: "PT1M"},
		{duration: DurationFromStandard(time.Second), expected: "PT1S"},
		{duration: NewDuration(DurationWithHours(math.MaxInt64)), expected: "PT9223372036854775807H"},
	} {
		t.Run(c.duration.String(), func(t *testing.T) {
			assert.Equal(t, c.expected, c.duration.String())
		})
	}
}

func TestParseDuration(t *testing.T) {
	for _, c := range []struct {
		s        string
		expected time.Duration
	}{
		{s: "P0D", expected: 0},
		{s: "P1D", expected: 24 * time.Hour},
		{s: "P1M", expected: iso8601.Month},
		{s: "P1Y", expected: iso8601.Year},
		{s: "P1Y1M1DT1H1M1.001S", expected: iso8601.Year +
			iso8601.Month +
			iso8601.Day +
			time.Hour +
			time.Minute +
			time.Second +
			time.Millisecond,
		},
		{s: "PT1S", expected: time.Second},
		{s: "PT1H", expected: time.Hour},
		{s: "PT1M", expected: time.Minute},
		{s: "PT1H1M1S", expected: time.Minute + time.Hour + time.Second},
		{s: "PT1S", expected: time.Second},
		{s: "PT0.001S", expected: time.Millisecond},
		{s: "-PT1H", expected: -time.Hour},
		{s: "+PT1H", expected: time.Hour},
	} {
		t.Run(c.s, func(t *testing.T) {
			v, err := ParseDuration(c.s)
			require.NoError(t, err)
			assert.Equal(t, float64(c.expected), v.Nanoseconds())
		})
	}
}

func TestDurationFromFloat64Nano(t *testing.T) {
	for _, c := range []struct {
		name string
		give float64
		want string
	}{
		{name: "should supports zero", give: 0, want: "P0D"},
		{name: "should not use unit above hour", give: float64(time.Hour * 24), want: "PT24H"},
		{
			name: "should supports positive",
			give: float64(time.Hour + time.Minute + time.Second + time.Nanosecond),
			want: "PT1H1M1.000000001S",
		},
		{
			name: "should supports negative",
			give: -float64(time.Hour + time.Minute + time.Second + time.Nanosecond),
			want: "-PT1H1M1.000000001S",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := DurationFromFloat64Nano(c.give)
			assert.Equal(t, c.give, got.Nanoseconds())
			assert.Equal(t, c.want, got.String())
		})
	}
}

func TestDurationTrunc(t *testing.T) {
	for _, c := range []struct {
		name      string
		giveInput Duration
		giveUnit  Duration
		want      string
	}{
		{name: "should supports zero", giveInput: MustParseDuration("P0D"), giveUnit: MustParseDuration("PT1H"), want: "P0D"},
		{name: "should supports positive", giveInput: MustParseDuration("PT3H59M59S"), giveUnit: MustParseDuration("PT1H"), want: "PT3H"},
		{name: "should supports negative", giveInput: MustParseDuration("-PT3H59M59S"), giveUnit: MustParseDuration("PT1H"), want: "-PT3H"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := c.giveInput.Trunc(c.giveUnit)
			assert.Equal(t, c.want, got.String())
		})
	}
}

func TestDurationCeil(t *testing.T) {
	for _, c := range []struct {
		name      string
		giveInput Duration
		giveUnit  Duration
		want      string
	}{
		{name: "should supports zero", giveInput: MustParseDuration("P0D"), giveUnit: MustParseDuration("PT1H"), want: "P0D"},
		{name: "should supports positive", giveInput: MustParseDuration("PT3H59M59S"), giveUnit: MustParseDuration("PT1H"), want: "PT4H"},
		{name: "should supports negative", giveInput: MustParseDuration("-PT3H59M59S"), giveUnit: MustParseDuration("PT1H"), want: "-PT3H"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := c.giveInput.Ceil(c.giveUnit)
			assert.Equal(t, c.want, got.String())
		})
	}
}

func TestDurationFloor(t *testing.T) {
	for _, c := range []struct {
		name      string
		giveInput Duration
		giveUnit  Duration
		want      string
	}{
		{name: "should supports zero", giveInput: MustParseDuration("P0D"), giveUnit: MustParseDuration("PT1H"), want: "P0D"},
		{name: "should supports positive", giveInput: MustParseDuration("PT3H59M59S"), giveUnit: MustParseDuration("PT1H"), want: "PT3H"},
		{name: "should supports negative", giveInput: MustParseDuration("-PT3H59M59S"), giveUnit: MustParseDuration("PT1H"), want: "-PT4H"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := c.giveInput.Floor(c.giveUnit)
			assert.Equal(t, c.want, got.String())
		})
	}
}

func BenchmarkParseDuration(b *testing.B) {
	x := "P1Y23M34W56DT78H90M12.3456789S"
	for i := 0; i < b.N; i++ {
		_, err := ParseDuration(x)
		if err != nil {
			b.Fatal(err)
		}
	}
}
