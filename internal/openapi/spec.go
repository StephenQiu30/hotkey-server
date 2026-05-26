package openapi

type Info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type Operation struct {
	Summary     string                    `json:"summary"`
	OperationID string                    `json:"operationId"`
	Responses   map[string]Response       `json:"responses"`
	Tags        []string                  `json:"tags,omitempty"`
	Extensions  map[string]map[string]any `json:"-"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema map[string]any `json:"schema"`
}

type PathItem struct {
	Get   Operation `json:"get,omitempty"`
	Post  Operation `json:"post,omitempty"`
	Patch Operation `json:"patch,omitempty"`
}

type Components struct {
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Description  string `json:"description,omitempty"`
}

type SpecDocument struct {
	OpenAPI    string                `json:"openapi"`
	Info       Info                  `json:"info"`
	Paths      map[string]PathItem   `json:"paths"`
	Components Components            `json:"components,omitempty"`
	Security   []map[string][]string `json:"security,omitempty"`
}

func Spec() SpecDocument {
	return SpecDocument{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:   "HotKey Server API",
			Version: "0.1.0",
		},
		Components: Components{
			SecuritySchemes: map[string]SecurityScheme{
				"BearerAuth": {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "JWT",
					Description:  "小程序和管理端调用受保护接口时使用 Authorization: Bearer <token>。",
				},
			},
		},
		Security: []map[string][]string{
			{"BearerAuth": {}},
		},
		Paths: map[string]PathItem{
			"/healthz": {
				Get: Operation{
					Summary:     "Service health check",
					OperationID: "getHealth",
					Tags:        []string{"system"},
					Responses: map[string]Response{
						"200": {
							Description: "Service is healthy",
							Content: map[string]MediaType{
								"application/json": {
									Schema: map[string]any{
										"type": "object",
										"properties": map[string]any{
											"status":  map[string]any{"type": "string"},
											"service": map[string]any{"type": "string"},
										},
										"required": []string{"status", "service"},
									},
								},
							},
						},
					},
				},
			},
			"/openapi.json": {
				Get: Operation{
					Summary:     "Export OpenAPI document",
					OperationID: "getOpenAPI",
					Tags:        []string{"system"},
					Responses: map[string]Response{
						"200": {
							Description: "OpenAPI document",
							Content: map[string]MediaType{
								"application/json": {
									Schema: map[string]any{"type": "object"},
								},
							},
						},
					},
				},
			},
			"/api/v1/admin/keywords": {
				Get: Operation{
					Summary:     "List platform keywords",
					OperationID: "listPlatformKeywords",
					Tags:        []string{"keyword"},
					Responses:   okObjectResponse("Platform keyword list"),
				},
				Post: Operation{
					Summary:     "Create platform keyword",
					OperationID: "createPlatformKeyword",
					Tags:        []string{"keyword"},
					Responses:   createdObjectResponse("Platform keyword created"),
				},
			},
			"/api/v1/admin/keywords/{id}": {
				Patch: Operation{
					Summary:     "Enable or disable platform keyword",
					OperationID: "setPlatformKeywordEnabled",
					Tags:        []string{"keyword"},
					Responses:   okObjectResponse("Platform keyword updated"),
				},
			},
			"/api/v1/admin/sources": {
				Get: Operation{
					Summary:     "List configured collection sources",
					OperationID: "listSources",
					Tags:        []string{"source"},
					Responses:   okObjectResponse("Source list"),
				},
			},
			"/api/v1/admin/sources/{id}": {
				Patch: Operation{
					Summary:     "Enable, disable, or throttle a collection source",
					OperationID: "updateSourceConfig",
					Tags:        []string{"source"},
					Responses:   okObjectResponse("Source configuration updated"),
				},
			},
			"/api/v1/admin/source-items": {
				Get: Operation{
					Summary:     "List normalized source items",
					OperationID: "listSourceItems",
					Tags:        []string{"content"},
					Responses:   okObjectResponse("Source item list"),
				},
				Post: Operation{
					Summary:     "Ingest and deduplicate a source item",
					OperationID: "ingestSourceItem",
					Tags:        []string{"content"},
					Responses:   createdObjectResponse("Source item ingested"),
				},
			},
			"/api/v1/admin/event-candidates": {
				Post: Operation{
					Summary:     "Upsert source item into a candidate event cluster",
					OperationID: "upsertEventCandidate",
					Tags:        []string{"event"},
					Responses:   createdObjectResponse("Event candidate clustered"),
				},
			},
			"/api/v1/admin/event-clusters": {
				Get: Operation{
					Summary:     "List candidate event clusters",
					OperationID: "listEventClusters",
					Tags:        []string{"event"},
					Responses:   okObjectResponse("Event cluster list"),
				},
			},
			"/api/v1/admin/event-evidence": {
				Post: Operation{
					Summary:     "Add fact or signal evidence to an event",
					OperationID: "addEventEvidence",
					Tags:        []string{"trust"},
					Responses:   createdObjectResponse("Event evidence added"),
				},
			},
			"/api/v1/admin/events/{id}/ai-summary": {
				Post: Operation{
					Summary:     "Set event AI summary with source citations",
					OperationID: "setEventAISummary",
					Tags:        []string{"trust"},
					Responses:   okObjectResponse("Event AI summary updated"),
				},
			},
			"/api/v1/admin/task-runs": {
				Get: Operation{
					Summary:     "List admin task run records",
					OperationID: "listAdminTaskRuns",
					Tags:        []string{"admin"},
					Responses:   okObjectResponse("Admin task run list"),
				},
			},
			"/api/v1/admin/reports/daily": {
				Post: Operation{
					Summary:     "Trigger daily report generation from admin console",
					OperationID: "triggerAdminDailyReport",
					Tags:        []string{"admin", "report"},
					Responses:   acceptedObjectResponse("Daily report generation accepted"),
				},
			},
			"/api/v1/admin/tenants": {
				Get: Operation{
					Summary:     "List tenant organization spaces",
					OperationID: "listTenants",
					Tags:        []string{"tenant"},
					Responses:   okObjectResponse("Tenant list"),
				},
				Post: Operation{
					Summary:     "Create tenant organization space",
					OperationID: "createTenant",
					Tags:        []string{"tenant"},
					Responses:   createdObjectResponse("Tenant created"),
				},
			},
			"/api/v1/admin/tenants/{id}/members": {
				Post: Operation{
					Summary:     "Add user membership to a tenant",
					OperationID: "addTenantMember",
					Tags:        []string{"tenant"},
					Responses:   createdObjectResponse("Tenant member added"),
				},
			},
			"/api/v1/admin/tenants/{id}/keywords": {
				Get: Operation{
					Summary:     "List tenant-scoped keywords",
					OperationID: "listTenantKeywords",
					Tags:        []string{"tenant", "keyword"},
					Responses:   okObjectResponse("Tenant keyword list"),
				},
				Post: Operation{
					Summary:     "Create tenant-scoped keyword",
					OperationID: "createTenantKeyword",
					Tags:        []string{"tenant", "keyword"},
					Responses:   createdObjectResponse("Tenant keyword created"),
				},
			},
			"/api/v1/admin/tenants/{id}/sources": {
				Get: Operation{
					Summary:     "List tenant-scoped sources",
					OperationID: "listTenantSources",
					Tags:        []string{"tenant", "source"},
					Responses:   okObjectResponse("Tenant source list"),
				},
				Post: Operation{
					Summary:     "Create tenant-scoped source",
					OperationID: "createTenantSource",
					Tags:        []string{"tenant", "source"},
					Responses:   createdObjectResponse("Tenant source created"),
				},
			},
			"/api/v1/admin/tenants/{id}/sources/{sourceId}": {
				Patch: Operation{
					Summary:     "Update tenant-scoped source",
					OperationID: "updateTenantSource",
					Tags:        []string{"tenant", "source"},
					Responses:   okObjectResponse("Tenant source updated"),
				},
			},
			"/api/v1/admin/tenants/{id}/roles": {
				Post: Operation{
					Summary:     "Grant RBAC role in tenant scope",
					OperationID: "grantTenantRole",
					Tags:        []string{"rbac"},
					Responses:   createdObjectResponse("Tenant role granted"),
				},
			},
			"/api/v1/admin/tenants/{id}/authorize": {
				Post: Operation{
					Summary:     "Evaluate tenant-scoped RBAC permission",
					OperationID: "authorizeTenantAction",
					Tags:        []string{"rbac"},
					Responses:   okObjectResponse("Tenant authorization result"),
				},
			},
			"/api/v1/admin/tenants/{id}/audit-logs": {
				Get: Operation{
					Summary:     "List tenant audit log events",
					OperationID: "listTenantAuditLogs",
					Tags:        []string{"audit"},
					Responses:   okObjectResponse("Tenant audit logs"),
				},
			},
			"/api/v1/admin/tenants/{id}/billing/plan": {
				Post: Operation{
					Summary:     "Assign tenant billing plan and quotas",
					OperationID: "assignTenantBillingPlan",
					Tags:        []string{"billing"},
					Responses:   okObjectResponse("Tenant billing plan assigned"),
				},
			},
			"/api/v1/admin/tenants/{id}/billing/usage": {
				Get: Operation{
					Summary:     "Get tenant usage summary",
					OperationID: "getTenantUsageSummary",
					Tags:        []string{"billing"},
					Responses:   okObjectResponse("Tenant usage summary"),
				},
				Post: Operation{
					Summary:     "Record tenant usage against quotas",
					OperationID: "recordTenantUsage",
					Tags:        []string{"billing"},
					Responses:   acceptedOrPaymentRequiredResponse("Tenant usage recorded"),
				},
			},
			"/api/v1/users/{id}/tenants": {
				Get: Operation{
					Summary:     "List tenant spaces for a user",
					OperationID: "listUserTenants",
					Tags:        []string{"tenant"},
					Responses:   okObjectResponse("User tenant list"),
				},
			},
			"/api/v1/events/{id}/evidence": {
				Get: Operation{
					Summary:     "Get event evidence detail",
					OperationID: "getEventEvidence",
					Tags:        []string{"trust"},
					Responses:   okObjectResponse("Event evidence detail"),
				},
			},
			"/api/v1/hotspots": {
				Get: Operation{
					Summary:     "List ranked hotspots",
					OperationID: "listHotspots",
					Tags:        []string{"hotspot"},
					Responses:   okObjectResponse("Hotspot list"),
				},
			},
			"/api/v1/hotspots/{id}": {
				Get: Operation{
					Summary:     "Get hotspot detail",
					OperationID: "getHotspotDetail",
					Tags:        []string{"hotspot"},
					Responses:   okObjectResponse("Hotspot detail"),
				},
			},
			"/api/v1/reports/daily": {
				Get: Operation{
					Summary:     "Get platform daily report",
					OperationID: "getPlatformDailyReport",
					Tags:        []string{"report"},
					Responses:   okObjectResponse("Platform daily report"),
				},
			},
			"/api/v1/users/{id}/reports/daily": {
				Get: Operation{
					Summary:     "Get user keyword daily report",
					OperationID: "getUserDailyReport",
					Tags:        []string{"report"},
					Responses:   okObjectResponse("User daily report"),
				},
			},
			"/api/v1/tenants/{id}/reports/daily": {
				Get: Operation{
					Summary:     "Get tenant-scoped daily report",
					OperationID: "getTenantDailyReport",
					Tags:        []string{"tenant", "report"},
					Responses:   okObjectResponse("Tenant daily report"),
				},
			},
			"/api/v1/refresh-queue": {
				Post: Operation{
					Summary:     "Enqueue a manual refresh request with rate limiting",
					OperationID: "enqueueRefresh",
					Tags:        []string{"redis"},
					Responses:   createdObjectResponse("Refresh request queued"),
				},
			},
			"/api/v1/admin/refresh-queue": {
				Get: Operation{
					Summary:     "List pending refresh queue items",
					OperationID: "listRefreshQueue",
					Tags:        []string{"redis"},
					Responses:   okObjectResponse("Refresh queue list"),
				},
			},
			"/api/v1/admin/redis/health": {
				Get: Operation{
					Summary:     "Get Redis infrastructure health",
					OperationID: "getRedisHealth",
					Tags:        []string{"redis"},
					Responses:   okObjectResponse("Redis health"),
				},
			},
			"/api/v1/admin/work-queue/jobs": {
				Get: Operation{
					Summary:     "List pending work queue jobs",
					OperationID: "listWorkQueueJobs",
					Tags:        []string{"queue"},
					Responses:   okObjectResponse("Work queue job list"),
				},
				Post: Operation{
					Summary:     "Enqueue prioritized async job",
					OperationID: "enqueueWorkQueueJob",
					Tags:        []string{"queue"},
					Responses:   createdObjectResponse("Work queue job enqueued"),
				},
			},
			"/api/v1/admin/work-queue/run": {
				Post: Operation{
					Summary:     "Run worker pool for pending jobs",
					OperationID: "runWorkQueue",
					Tags:        []string{"queue"},
					Responses:   okObjectResponse("Worker pool result"),
				},
			},
			"/api/v1/admin/work-queue/compensations": {
				Get: Operation{
					Summary:     "List failed job compensations",
					OperationID: "listWorkQueueCompensations",
					Tags:        []string{"queue"},
					Responses:   okObjectResponse("Compensation list"),
				},
			},
			"/api/v1/admin/service-boundaries": {
				Get: Operation{
					Summary:     "Get API and Worker service split boundaries",
					OperationID: "getServiceBoundaries",
					Tags:        []string{"infra"},
					Responses:   okObjectResponse("Service boundary topology and task message contract"),
				},
			},
			"/api/v1/keywords/follow": {
				Post: Operation{
					Summary:     "Follow keyword for a user",
					OperationID: "followKeyword",
					Tags:        []string{"keyword"},
					Responses:   okObjectResponse("Keyword followed"),
				},
			},
			"/api/v1/keywords/block": {
				Post: Operation{
					Summary:     "Block keyword for a user",
					OperationID: "blockKeyword",
					Tags:        []string{"keyword"},
					Responses:   okObjectResponse("Keyword blocked"),
				},
			},
			"/api/v1/keywords/additional": {
				Post: Operation{
					Summary:     "Add user-specific keyword",
					OperationID: "addUserKeyword",
					Tags:        []string{"keyword"},
					Responses:   okObjectResponse("Keyword added"),
				},
			},
			"/api/v1/keywords/preferences": {
				Get: Operation{
					Summary:     "Get user keyword preferences",
					OperationID: "getUserKeywordPreferences",
					Tags:        []string{"keyword"},
					Responses:   okObjectResponse("User keyword preferences"),
				},
			},
		},
	}
}

func okObjectResponse(description string) map[string]Response {
	return map[string]Response{
		"200": objectResponse(description),
		"400": errorResponse(),
		"401": errorResponse(),
	}
}

func createdObjectResponse(description string) map[string]Response {
	return map[string]Response{
		"201": objectResponse(description),
		"400": errorResponse(),
		"401": errorResponse(),
	}
}

func acceptedObjectResponse(description string) map[string]Response {
	return map[string]Response{
		"202": objectResponse(description),
		"400": errorResponse(),
		"401": errorResponse(),
	}
}

func acceptedOrPaymentRequiredResponse(description string) map[string]Response {
	return map[string]Response{
		"202": objectResponse(description),
		"400": errorResponse(),
		"401": errorResponse(),
		"402": objectResponse("Quota exceeded"),
	}
}

func objectResponse(description string) Response {
	return Response{
		Description: description,
		Content: map[string]MediaType{
			"application/json": {
				Schema: map[string]any{"type": "object"},
			},
		},
	}
}

func errorResponse() Response {
	return Response{
		Description: "Structured error response",
		Content: map[string]MediaType{
			"application/json": {
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"code":    map[string]any{"type": "string"},
								"message": map[string]any{"type": "string"},
							},
							"required": []string{"code", "message"},
						},
					},
					"required": []string{"error"},
				},
			},
		},
	}
}
