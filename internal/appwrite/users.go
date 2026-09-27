package appwrite

import (
	"context"
	"net/http"
)

// Users endpoints and field names verified against github.com/appwrite/appwrite
// tag 2.3.0:
//
//   - GET /v1/users (scope users.read)
//     src/Appwrite/Platform/Modules/Users/Http/Users/XList.php
//
// List response envelope is {"total": int, "users": [...]}, same
// convention as every other list endpoint in this package; field names
// come from Model/User.php.
//
// User deliberately omits several fields the raw API can return:
// password/hash/hashOptions (credential material — must never be
// persisted, full stop) and email/phone/prefs (personally identifiable
// or arbitrary application data amg has no need to read for structural
// verification). See docs/migration-semantics.md for the reasoning.
type User struct {
	ID                string   `json:"$id"`
	CreatedAt         string   `json:"$createdAt"`
	UpdatedAt         string   `json:"$updatedAt"`
	Name              string   `json:"name"`
	Status            bool     `json:"status"`
	Labels            []string `json:"labels"`
	EmailVerification bool     `json:"emailVerification"`
	PhoneVerification bool     `json:"phoneVerification"`
	MFA               bool     `json:"mfa"`
	Registration      string   `json:"registration"`
}

type userListResponse struct {
	Total int    `json:"total"`
	Users []User `json:"users"`
}

// ListUsers returns every user in the project, fully paginated and
// ordered by $id for deterministic output. It decodes only the fields
// declared on User — password/hash/email/phone/prefs, even if present in
// the raw response, are never unmarshaled into anything amg retains.
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	const op = "appwrite.ListUsers"
	return paginate(ctx, op, func(u User) string { return u.ID }, func(ctx context.Context, cursor string) ([]User, error) {
		queries := []string{Limit(pageSize), OrderAsc("$id")}
		if cursor != "" {
			queries = append(queries, CursorAfter(cursor))
		}
		var out userListResponse
		if err := c.request(ctx, op, http.MethodGet, "/users", QueryParams(queries...), nil, &out); err != nil {
			return nil, err
		}
		return out.Users, nil
	})
}
