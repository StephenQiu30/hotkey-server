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

type SpecDocument struct {
	OpenAPI string              `json:"openapi"`
	Info    Info                `json:"info"`
	Paths   map[string]PathItem `json:"paths"`
}

func Spec() SpecDocument {
	return SpecDocument{
		OpenAPI: "3.1.0",
		Info: Info{
			Title:   "HotKey Server API",
			Version: "0.1.0",
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
	}
}

func createdObjectResponse(description string) map[string]Response {
	return map[string]Response{
		"201": objectResponse(description),
		"400": errorResponse(),
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
