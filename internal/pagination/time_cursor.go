package pagination

import (
	"strconv"
	"time"
)

func TimeToCursor(v time.Time) string {
	return strconv.FormatInt(v.UnixMicro(), 16)
}

func TimeFromCursor(v string) (time.Time, error) {
	var ms, err = strconv.ParseInt(v, 16, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMicro(ms), nil
}
