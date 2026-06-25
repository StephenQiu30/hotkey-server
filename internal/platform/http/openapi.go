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
