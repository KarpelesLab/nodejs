package nodejs

import "errors"

var (
	ErrTimeout = errors.New("nodejs: timeout reached")
)
