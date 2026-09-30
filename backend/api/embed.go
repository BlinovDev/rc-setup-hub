// Package api contains the committed frontend API contract.
package api

import _ "embed"

// OpenAPI is embedded verbatim; the YAML file is the source of truth.
//
//go:embed openapi.yaml
var OpenAPI []byte
