package http

// BuildOpenAPISpec returns a static OpenAPI 3.1.0 document matching registered routes.
func BuildOpenAPISpec() map[string]any {
	bearerSecurity := []map[string][]string{{"bearer": {}}}
	userContent := schemaContent("#/components/schemas/UserEnvelope")
	loginContent := schemaContent("#/components/schemas/LoginEnvelope")
	monitorContent := schemaContent("#/components/schemas/MonitorEnvelope")
	monitorListContent := schemaContent("#/components/schemas/MonitorListEnvelope")
	postListContent := schemaContent("#/components/schemas/PostListEnvelope")
	topicListContent := schemaContent("#/components/schemas/TopicListEnvelope")
	trendListContent := schemaContent("#/components/schemas/TrendListEnvelope")
	notificationListContent := schemaContent("#/components/schemas/NotificationListEnvelope")
	markNotificationReadContent := schemaContent("#/components/schemas/MarkNotificationReadEnvelope")

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
				"HealthBody": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status": map[string]any{"type": "string"},
					},
					"required": []string{"status"},
				},
				"HealthEnvelope": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data":       map[string]any{"$ref": "#/components/schemas/HealthBody"},
						"request_id": map[string]any{"type": "string"},
					},
					"required": []string{"data"},
				},
				"ResponseEnvelope": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"data":       map[string]any{},
						"request_id": map[string]any{"type": "string"},
					},
					"required": []string{"data"},
				},
				"UserEnvelope": envelopeSchema(refSchema("#/components/schemas/UserResponse")),
				"UserResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":           map[string]any{"type": "integer", "format": "int64"},
						"email":        map[string]any{"type": "string"},
						"display_name": map[string]any{"type": "string"},
					},
				},
				"LoginEnvelope": envelopeSchema(refSchema("#/components/schemas/LoginResponse")),
				"LoginResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"user":  map[string]any{"$ref": "#/components/schemas/UserResponse"},
						"token": map[string]any{"type": "string"},
					},
				},
				"MonitorEnvelope":     envelopeSchema(refSchema("#/components/schemas/MonitorResponse")),
				"MonitorListEnvelope": envelopeSchema(arraySchema("#/components/schemas/MonitorResponse")),
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
				"PostListEnvelope": envelopeSchema(arraySchema("#/components/schemas/PostSummary")),
				"PostSummary": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":               map[string]any{"type": "integer", "format": "int64"},
						"platform_post_id": map[string]any{"type": "string"},
						"author_name":      map[string]any{"type": "string"},
						"author_handle":    map[string]any{"type": "string"},
						"content_text":     map[string]any{"type": "string"},
						"content_lang":     map[string]any{"type": "string"},
						"published_at":     map[string]any{"type": "string"},
						"like_count":       map[string]any{"type": "integer"},
						"reply_count":      map[string]any{"type": "integer"},
						"repost_count":     map[string]any{"type": "integer"},
						"quote_count":      map[string]any{"type": "integer"},
						"view_count":       map[string]any{"type": "integer"},
						"heat_score":       map[string]any{"type": "number"},
						"relevance_score":  map[string]any{"type": "number"},
						"freshness_score":  map[string]any{"type": "number"},
						"final_score":      map[string]any{"type": "number"},
						"matched_keywords": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
				},
				"TopicListEnvelope": envelopeSchema(arraySchema("#/components/schemas/TopicSummary")),
				"TopicSummary": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":              map[string]any{"type": "integer", "format": "int64"},
						"title":           map[string]any{"type": "string"},
						"summary":         map[string]any{"type": "string"},
						"current_heat":    map[string]any{"type": "number"},
						"trend_direction": map[string]any{"type": "string"},
						"post_count":      map[string]any{"type": "integer"},
					},
				},
				"TrendListEnvelope": envelopeSchema(arraySchema("#/components/schemas/TrendPoint")),
				"TrendPoint": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"time":            map[string]any{"type": "string", "format": "date-time"},
						"heat_score":      map[string]any{"type": "number"},
						"trend_velocity":  map[string]any{"type": "number"},
						"trend_direction": map[string]any{"type": "string"},
					},
				},
				"NotificationListEnvelope": envelopeSchema(arraySchema("#/components/schemas/NotificationResponse")),
				"NotificationResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":              map[string]any{"type": "integer", "format": "int64"},
						"user_id":         map[string]any{"type": "integer", "format": "int64"},
						"alert_id":        map[string]any{"type": "integer", "format": "int64"},
						"channel":         map[string]any{"type": "string"},
						"delivery_status": map[string]any{"type": "string"},
						"read_at":         map[string]any{"type": "string", "format": "date-time"},
						"created_at":      map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"MarkNotificationReadEnvelope": envelopeSchema(refSchema("#/components/schemas/MarkNotificationReadResponse")),
				"MarkNotificationReadResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"read": map[string]any{"type": "boolean"},
					},
					"required": []string{"read"},
				},
			},
		},
		"paths": map[string]any{
			"/healthz": map[string]any{
				"get": op("health-check", "Health check", []string{"health"}, nil, schemaContent("#/components/schemas/HealthEnvelope")),
			},
			"/api/v1/auth/register": map[string]any{
				"post": opCreated("register", "Register a new user", []string{"auth"}, nil, userContent),
			},
			"/api/v1/auth/login": map[string]any{
				"post": op("login", "Login with email and password", []string{"auth"}, nil, loginContent),
			},
			"/api/v1/monitors": map[string]any{
				"get":  op("list-monitors", "List monitors", []string{"monitors"}, bearerSecurity, monitorListContent),
				"post": opCreated("create-monitor", "Create a monitor", []string{"monitors"}, bearerSecurity, monitorContent),
			},
			"/api/v1/monitors/{id}": map[string]any{
				"get":   op("get-monitor", "Get a monitor", []string{"monitors"}, bearerSecurity, monitorContent),
				"patch": op("update-monitor", "Update a monitor", []string{"monitors"}, bearerSecurity, monitorContent),
			},
			"/api/v1/monitors/{id}/posts": map[string]any{
				"get": op("list-posts", "List posts for a monitor", []string{"content"}, bearerSecurity, postListContent),
			},
			"/api/v1/monitors/{id}/topics": map[string]any{
				"get": op("list-topics", "List topics for a monitor", []string{"topics"}, bearerSecurity, topicListContent),
			},
			"/api/v1/monitors/{id}/trends": map[string]any{
				"get": op("get-monitor-trends", "Get trends for a monitor", []string{"trends"}, bearerSecurity, trendListContent),
			},
			"/api/v1/topics/{id}/trends": map[string]any{
				"get": op("get-topic-trends", "Get trends for a topic", []string{"trends"}, bearerSecurity, trendListContent),
			},
			"/api/v1/notifications": map[string]any{
				"get": op("list-notifications", "List unread notifications", []string{"notifications"}, bearerSecurity, notificationListContent),
			},
			"/api/v1/notifications/{id}/read": map[string]any{
				"post": op("mark-notification-read", "Mark notification as read", []string{"notifications"}, bearerSecurity, markNotificationReadContent),
			},
		},
	}
}

func envelopeSchema(dataSchema map[string]any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data":       dataSchema,
			"request_id": map[string]any{"type": "string"},
		},
		"required": []string{"data"},
	}
}

func refSchema(ref string) map[string]any {
	return map[string]any{"$ref": ref}
}

func arraySchema(itemRef string) map[string]any {
	return map[string]any{
		"type":  "array",
		"items": refSchema(itemRef),
	}
}

func schemaContent(ref string) map[string]any {
	return map[string]any{
		"application/json": map[string]any{
			"schema": map[string]any{"$ref": ref},
		},
	}
}

func op(operationID, summary string, tags []string, security []map[string][]string, content map[string]any) map[string]any {
	return opWithStatus(operationID, summary, tags, security, content, "200", "OK")
}

func opCreated(operationID, summary string, tags []string, security []map[string][]string, content map[string]any) map[string]any {
	return opWithStatus(operationID, summary, tags, security, content, "201", "Created")
}

func opWithStatus(operationID, summary string, tags []string, security []map[string][]string, content map[string]any, successStatus, successDescription string) map[string]any {
	m := map[string]any{
		"operationId": operationID,
		"summary":     summary,
		"tags":        tags,
	}
	if security != nil {
		m["security"] = security
	}
	successResponse := map[string]any{"description": successDescription}
	if content != nil {
		successResponse["content"] = content
	}
	m["responses"] = map[string]any{
		successStatus: successResponse,
		"default":     errorResponse(),
	}
	return m
}

func errorResponse() map[string]any {
	return map[string]any{
		"description": "Error",
		"content":     schemaContent("#/components/schemas/ErrorBody"),
	}
}
