package e2e_test

// SimulatorError represents an error returned by a provider simulator.
type SimulatorError struct {
	Code    string
	Message string
}

// Error returns the string representation of the simulator error.
func (e *SimulatorError) Error() string {
	return e.Code + ": " + e.Message
}

// NewSimulatorError creates a new instance of SimulatorError with the given code and message.
func NewSimulatorError(code, message string) *SimulatorError {
	return &SimulatorError{Code: code, Message: message}
}
