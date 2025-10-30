package azpim

import (
	"errors"
)

var (
	// ErrNoItems is returned when there are no items to select from
	ErrNoItems = errors.New("no items to select from")
	// ErrUserCancelled is returned when user cancels the operation
	ErrUserCancelled = errors.New("user cancelled")
	// ErrInvalidHours is returned when hours is out of valid range
	ErrInvalidHours = errors.New("hours must be between 1 and 8")
)
