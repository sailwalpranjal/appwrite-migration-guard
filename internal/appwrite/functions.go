package appwrite

import (
	"context"
	"net/http"
)

// Functions endpoints and field names verified against
// github.com/appwrite/appwrite tag 2.3.0:
//
//   - GET /v1/functions (scope functions.read)
//     src/Appwrite/Platform/Modules/Functions/Http/Functions/XList.php
//
// List response envelope is {"total": int, "functions": [...]}, same
// convention as every other list endpoint in this package; field names
// come from Model/Func.php (named Func, not Function, to avoid the PHP
// reserved word).
//
// Function deliberately omits "vars": Appwrite's own model documents it
// as "Function variables" (MODEL_VARIABLE, array) — function environment
// variables routinely hold secrets (API keys, database passwords,
// tokens). There is no field on this struct for them to decode into, the
// same pattern used for User's password/hash/email/phone.
type Function struct {
	ID                  string   `json:"$id"`
	CreatedAt           string   `json:"$createdAt"`
	UpdatedAt           string   `json:"$updatedAt"`
	Execute             []string `json:"execute"`
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	Logging             bool     `json:"logging"`
	Runtime             string   `json:"runtime"`
	DeploymentRetention int      `json:"deploymentRetention"`
	Scopes              []string `json:"scopes"`
	Events              []string `json:"events"`
	Schedule            string   `json:"schedule"`
	Timeout             int      `json:"timeout"`
	Entrypoint          string   `json:"entrypoint"`
	Version             string   `json:"version"`
}

type functionListResponse struct {
	Total     int        `json:"total"`
	Functions []Function `json:"functions"`
}

// ListFunctions returns every function in the project, fully paginated
// and ordered by $id for deterministic output. Environment variables
// ("vars") are never fetched or decoded — see the Function doc comment.
func (c *Client) ListFunctions(ctx context.Context) ([]Function, error) {
	const op = "appwrite.ListFunctions"
	return paginate(ctx, op, func(f Function) string { return f.ID }, func(ctx context.Context, cursor string) ([]Function, error) {
		queries := []string{Limit(pageSize), OrderAsc("$id")}
		if cursor != "" {
			queries = append(queries, CursorAfter(cursor))
		}
		var out functionListResponse
		if err := c.request(ctx, op, http.MethodGet, "/functions", QueryParams(queries...), nil, &out); err != nil {
			return nil, err
		}
		return out.Functions, nil
	})
}
