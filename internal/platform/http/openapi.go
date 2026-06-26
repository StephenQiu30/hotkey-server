package http

// BuildOpenAPISpec returns a static OpenAPI 3.1.0 document matching registered routes.
func BuildOpenAPISpec() map[string]any {
	bearerSecurity := []map[string][]string{{"bearer": {}}}
	jsonContent := map[string]any{"application/json": map[string]any{}}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "HotKey Server",
			"version":     "1.0.0",
			"description": "X (Twitter) hot-topic monitoring platform API",
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearer": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
			"schemas": map[string]any{
				"ErrorBody": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error":      map[string]any{"type": "string"},
						"code":       map[string]any{"type": "string"},
						"request_id": map[string]any{"type": "string"},
					},
					"required": []string{"error"},
				},
				"UserResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":           map[string]any{"type": "integer", "format": "int64"},
						"email":        map[string]any{"type": "string"},
						"display_name": map[string]any{"type": "string"},
					},
				},
				"LoginResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"user":  map[string]any{"$ref": "#/components/schemas/UserResponse"},
						"token": map[string]any{"type": "string"},
					},
				},
				"MonitorResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":                    map[string]any{"type": "integer", "format": "int64"},
						"user_id":               map[string]any{"type": "integer", "format": "int64"},
						"name":                  map[string]any{"type": "string"},
						"query_text":            map[string]any{"type": "string"},
						"status":                map[string]any{"type": "string"},
						"poll_interval_minutes": map[string]any{"type": "integer"},
						"alert_enabled":         map[string]any{"type": "boolean"},
					},
				},
				"NotificationResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":              map[string]any{"type": "integer", "format": "int64"},
						"channel":         map[string]any{"type": "string"},
						"delivery_status": map[string]any{"type": "string"},
						"created_at":      map[string]any{"type": "string", "format": "date-time"},
					},
				},
			},
		},
		"paths": map[string]any{
			"/healthz": map[string]any{
				"get": op("health-check", "Health check", []string{"health"}, nil, nil),
			},
			"/api/v1/auth/register": map[string]any{
				"post": op("register", "Register a new user", []string{"auth"}, nil, jsonContent),
			},
			"/api/v1/auth/login": map[string]any{
				"post": op("login", "Login with email and password", []string{"auth"}, nil, jsonContent),
			},
			"/api/v1/monitors": map[string]any{
				"get":  op("list-monitors", "List monitors", []string{"monitors"}, bearerSecurity, jsonContent),
				"post": op("create-monitor", "Create a monitor", []string{"monitors"}, bearerSecurity, jsonContent),
			},
			"/api/v1/monitors/{id}": map[string]any{
				"get":   op("get-monitor", "Get a monitor", []string{"monitors"}, bearerSecurity, jsonContent),
				"patch": op("update-monitor", "Update a monitor", []string{"monitors"}, bearerSecurity, jsonContent),
			},
			"/api/v1/monitors/{id}/posts": map[string]any{
				"get": op("list-posts", "List posts for a monitor", []string{"content"}, bearerSecurity, jsonContent),
			},
			"/api/v1/monitors/{id}/topics": map[string]any{
				"get": op("list-topics", "List topics for a monitor", []string{"topics"}, bearerSecurity, jsonContent),
			},
			"/api/v1/monitors/{id}/trends": map[string]any{
				"get": op("get-monitor-trends", "Get trends for a monitor", []string{"trends"}, bearerSecurity, jsonContent),
			},
			"/api/v1/topics/{id}/trends": map[string]any{
				"get": op("get-topic-trends", "Get trends for a topic", []string{"trends"}, bearerSecurity, jsonContent),
			},
			"/api/v1/notifications": map[string]any{
				"get": op("list-notifications", "List unread notifications", []string{"notifications"}, bearerSecurity, jsonContent),
			},
			"/api/v1/notifications/{id}/read": map[string]any{
				"post": op("mark-notification-read", "Mark notification as read", []string{"notifications"}, bearerSecurity, jsonContent),
			},
		},
	}
}

func op(operationID, summary string, tags []string, security []map[string][]string, content map[string]any) map[string]any {
	m := map[string]any{
		"operationId": operationID,
		"summary":     summary,
		"tags":        tags,
	}
	if security != nil {
		m["security"] = security
	}
	if content != nil {
		m["responses"] = map[string]any{
			"200": map[string]any{"description": "OK", "content": content},
		}
	} else {
		m["responses"] = map[string]any{
			"200": map[string]any{"description": "OK"},
		}
	}
	return m
}
