package util

import (
	"fmt"
)

// ErrorfIf returns nil, if err is nil
func ErrorfIf(format string, err error) error {
	if err != nil {
		return fmt.Errorf(format, err)
	}

	return nil
}
