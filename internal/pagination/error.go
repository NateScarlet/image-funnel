package pagination

import "errors"

var ErrFirstOrLastMissing = errors.New("pagination: must specify one of (first, last)")
var ErrFirstAndLastAtSameTime = errors.New("pagination: use first and last at same time is not supported")
var ErrNegativePageSize = errors.New("pagination: negative page size is not allowed")
