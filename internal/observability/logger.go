package observability

import "fmt"

// RenderLog produces a structured JSON log line with service and message fields.
func RenderLog(service, message string) string {
	return fmt.Sprintf(`{"service":"%s","message":"%s"}`, service, message)
}
