package service

import "time"

// RealClock implements the Clock interface using time.Now.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }
