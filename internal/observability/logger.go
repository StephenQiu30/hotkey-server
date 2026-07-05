package observability

import "encoding/json"

// RenderLog produces a structured JSON log line with service and message fields.
func RenderLog(service, message string) string {
	data, _ := json.Marshal(map[string]string{
		"service": service,
		"message": message,
	})
	return string(data)
}
