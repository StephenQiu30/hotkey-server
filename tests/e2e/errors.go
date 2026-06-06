package e2e_test

// SimulatorError represents an error returned by a provider simulator.
type SimulatorError struct {
	Code    string
	Message string
}

func (e *SimulatorError) Error() string {
	return e.Code + ": " + e.Message
}

func NewSimulatorError(code, message string) *SimulatorError {
	return &SimulatorError{Code: code, Message: message}
}
