package appwrite

import (
	"context"
	"net/http"
)

// Sites endpoints and field names verified against
// github.com/appwrite/appwrite tag 2.3.0:
//
//   - GET /v1/sites (scope sites.read)
//     src/Appwrite/Platform/Modules/Sites/Http/Sites/XList.php
//     (queries the internal "sites" collection directly)
//
// List response envelope is {"total": int, "sites": [...]}; field names
// come from Model/Site.php.
//
// Site deliberately omits "vars": exactly like Function, Appwrite's own
// model documents it as MODEL_VARIABLE (environment variables), which
// routinely hold build-time secrets. There is no field on this struct
// for them to decode into. Sites also have no $permissions/execute
// field in the raw model at all (unlike tables/buckets/functions) —
// they are typically public-facing static sites, not access-controlled
// resources, so Resource.Permissions is left empty for this type.
type Site struct {
	ID                  string   `json:"$id"`
	CreatedAt           string   `json:"$createdAt"`
	UpdatedAt           string   `json:"$updatedAt"`
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	Logging             bool     `json:"logging"`
	Framework           string   `json:"framework"`
	DeploymentRetention int      `json:"deploymentRetention"`
	Scopes              []string `json:"scopes"`
	Timeout             int      `json:"timeout"`
	InstallCommand      string   `json:"installCommand"`
	BuildCommand        string   `json:"buildCommand"`
	StartCommand        string   `json:"startCommand"`
	OutputDirectory     string   `json:"outputDirectory"`
	BuildRuntime        string   `json:"buildRuntime"`
	Adapter             string   `json:"adapter"`
	FallbackFile        string   `json:"fallbackFile"`
}

type siteListResponse struct {
	Total int    `json:"total"`
	Sites []Site `json:"sites"`
}

// ListSites returns every site in the project, fully paginated and
// ordered by $id for deterministic output. Environment variables
// ("vars") are never fetched or decoded — see the Site doc comment.
func (c *Client) ListSites(ctx context.Context) ([]Site, error) {
	const op = "appwrite.ListSites"
	return paginate(ctx, op, func(s Site) string { return s.ID }, func(ctx context.Context, cursor string) ([]Site, error) {
		queries := []string{Limit(pageSize), OrderAsc("$id")}
		if cursor != "" {
			queries = append(queries, CursorAfter(cursor))
		}
		var out siteListResponse
		if err := c.request(ctx, op, http.MethodGet, "/sites", QueryParams(queries...), nil, &out); err != nil {
			return nil, err
		}
		return out.Sites, nil
	})
}
