package appwrite

import (
	"context"
	"fmt"
	"net/http"
)

// TablesDB is Appwrite's current database API (tables/rows), which
// replaced the legacy "Databases" API (collections/documents) that
// remains present for backward compatibility. Endpoints, scopes, and
// field names below are verified against github.com/appwrite/appwrite
// tag 2.3.0:
//
//   - GET /v1/tablesdb                                  (scope databases.read)
//     src/Appwrite/Platform/Modules/Databases/Http/TablesDB/XList.php
//   - GET /v1/tablesdb/:databaseId/tables               (scope tables.read|collections.read)
//     .../TablesDB/Tables/XList.php
//   - GET /v1/tablesdb/:databaseId/tables/:tableId/rows (scope rows.read|documents.read)
//     .../TablesDB/Tables/Rows/XList.php
//
// List response envelopes are {"total": int, "<key>": [...]}
// (src/Appwrite/Utopia/Response/Model/BaseList.php); field names on each
// resource come from Model/Database.php, Model/Table.php, Model/Row.php.

// Database is one Appwrite TablesDB database.
type Database struct {
	ID        string `json:"$id"`
	Name      string `json:"name"`
	CreatedAt string `json:"$createdAt"`
	UpdatedAt string `json:"$updatedAt"`
	Enabled   bool   `json:"enabled"`
	Type      string `json:"type"`
	Status    string `json:"status"`
}

type databaseListResponse struct {
	Total     int        `json:"total"`
	Databases []Database `json:"databases"`
}

// Table is one table within a TablesDB database.
type Table struct {
	ID          string   `json:"$id"`
	CreatedAt   string   `json:"$createdAt"`
	UpdatedAt   string   `json:"$updatedAt"`
	Permissions []string `json:"$permissions"`
	DatabaseID  string   `json:"databaseId"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	RowSecurity bool     `json:"rowSecurity"`
	BytesMax    int64    `json:"bytesMax"`
	BytesUsed   int64    `json:"bytesUsed"`
}

type tableListResponse struct {
	Total  int     `json:"total"`
	Tables []Table `json:"tables"`
}

type rowListResponse struct {
	Total int              `json:"total"`
	Rows  []map[string]any `json:"rows"`
}

// rowCountCap is the point at which Appwrite stops computing an exact
// total and caps it, per BaseList.php: "When the total number of ... rows
// available is greater than 5000, total returned will be capped at
// 5000, and cursor pagination should be used." A CountRows result of
// exactly this value is therefore a floor, not an exact count.
const rowCountCap = 5000

// ListDatabases returns every database in the project, fully paginated
// and ordered by $id for deterministic output.
func (c *Client) ListDatabases(ctx context.Context) ([]Database, error) {
	const op = "appwrite.ListDatabases"
	return paginate(ctx, op, func(d Database) string { return d.ID }, func(ctx context.Context, cursor string) ([]Database, error) {
		queries := []string{Limit(pageSize), OrderAsc("$id")}
		if cursor != "" {
			queries = append(queries, CursorAfter(cursor))
		}
		var out databaseListResponse
		if err := c.request(ctx, op, http.MethodGet, "/tablesdb", QueryParams(queries...), nil, &out); err != nil {
			return nil, err
		}
		return out.Databases, nil
	})
}

// ListTables returns every table in databaseID, fully paginated and
// ordered by $id.
func (c *Client) ListTables(ctx context.Context, databaseID string) ([]Table, error) {
	op := fmt.Sprintf("appwrite.ListTables(%s)", databaseID)
	return paginate(ctx, op, func(t Table) string { return t.ID }, func(ctx context.Context, cursor string) ([]Table, error) {
		queries := []string{Limit(pageSize), OrderAsc("$id")}
		if cursor != "" {
			queries = append(queries, CursorAfter(cursor))
		}
		var out tableListResponse
		path := fmt.Sprintf("/tablesdb/%s/tables", databaseID)
		if err := c.request(ctx, op, http.MethodGet, path, QueryParams(queries...), nil, &out); err != nil {
			return nil, err
		}
		return out.Tables, nil
	})
}

// CountRows returns the number of rows in a table without downloading row
// content: it requests a single row (limit 1) and reads the server-computed
// "total". capped reports whether the count hit rowCountCap, meaning the
// true count may be higher (see rowCountCap doc comment) — callers should
// surface this as a partial-verification warning, not an exact number.
func (c *Client) CountRows(ctx context.Context, databaseID, tableID string) (count int, capped bool, err error) {
	op := fmt.Sprintf("appwrite.CountRows(%s/%s)", databaseID, tableID)
	path := fmt.Sprintf("/tablesdb/%s/tables/%s/rows", databaseID, tableID)
	var out rowListResponse
	if err := c.request(ctx, op, http.MethodGet, path, QueryParams(Limit(1)), nil, &out); err != nil {
		return 0, false, err
	}
	return out.Total, out.Total >= rowCountCap, nil
}
