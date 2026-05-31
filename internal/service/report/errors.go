package report

import "errors"

func errorsIs(err error, target error) bool {
	return errors.Is(err, target)
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
