package openapi

import _ "embed"

// Document is the generated public HTTP contract embedded into the server
// binary so the runtime documentation always matches the committed artifact.
//
//go:embed swagger.json
var Document []byte
