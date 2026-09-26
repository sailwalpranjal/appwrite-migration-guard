package appwrite

import (
	"context"
	"fmt"
	"net/http"
)

// Storage endpoints, scopes, and field names below are verified against
// github.com/appwrite/appwrite tag 2.3.0:
//
//   - GET /v1/storage/buckets                    (scope buckets.read)
//     src/Appwrite/Platform/Modules/Storage/Http/Buckets/XList.php
//   - GET /v1/storage/buckets/:bucketId/files     (scope files.read)
//     .../Storage/Http/Buckets/Files/XList.php
//
// List response envelopes are {"total": int, "<key>": [...]}, same as
// TablesDB (see tablesdb.go); field names come from
// Model/Bucket.php and Model/File.php.

// Bucket is one Appwrite Storage bucket.
type Bucket struct {
	ID                    string   `json:"$id"`
	CreatedAt             string   `json:"$createdAt"`
	UpdatedAt             string   `json:"$updatedAt"`
	Permissions           []string `json:"$permissions"`
	FileSecurity          bool     `json:"fileSecurity"`
	Name                  string   `json:"name"`
	Enabled               bool     `json:"enabled"`
	MaximumFileSize       int64    `json:"maximumFileSize"`
	AllowedFileExtensions []string `json:"allowedFileExtensions"`
	Compression           string   `json:"compression"`
	Encryption            bool     `json:"encryption"`
	Antivirus             bool     `json:"antivirus"`
}

type bucketListResponse struct {
	Total   int      `json:"total"`
	Buckets []Bucket `json:"buckets"`
}

// File is one file within a Storage bucket. Signature is Appwrite's own
// MD5 of the file content (Model/File.php: "File MD5 signature") — it
// lets amg verify content integrity by comparing this field, without ever
// downloading file bytes.
type File struct {
	ID           string   `json:"$id"`
	BucketID     string   `json:"bucketId"`
	CreatedAt    string   `json:"$createdAt"`
	UpdatedAt    string   `json:"$updatedAt"`
	Permissions  []string `json:"$permissions"`
	Name         string   `json:"name"`
	Signature    string   `json:"signature"`
	MimeType     string   `json:"mimeType"`
	SizeOriginal int64    `json:"sizeOriginal"`
}

type fileListResponse struct {
	Total int    `json:"total"`
	Files []File `json:"files"`
}

// ListBuckets returns every storage bucket in the project, fully
// paginated and ordered by $id.
func (c *Client) ListBuckets(ctx context.Context) ([]Bucket, error) {
	const op = "appwrite.ListBuckets"
	return paginate(ctx, op, func(b Bucket) string { return b.ID }, func(ctx context.Context, cursor string) ([]Bucket, error) {
		queries := []string{Limit(pageSize), OrderAsc("$id")}
		if cursor != "" {
			queries = append(queries, CursorAfter(cursor))
		}
		var out bucketListResponse
		if err := c.request(ctx, op, http.MethodGet, "/storage/buckets", QueryParams(queries...), nil, &out); err != nil {
			return nil, err
		}
		return out.Buckets, nil
	})
}

// ListFiles returns every file in bucketID, fully paginated and ordered
// by $id. It returns file metadata only, including the MD5 signature —
// never file content.
func (c *Client) ListFiles(ctx context.Context, bucketID string) ([]File, error) {
	op := fmt.Sprintf("appwrite.ListFiles(%s)", bucketID)
	return paginate(ctx, op, func(f File) string { return f.ID }, func(ctx context.Context, cursor string) ([]File, error) {
		queries := []string{Limit(pageSize), OrderAsc("$id")}
		if cursor != "" {
			queries = append(queries, CursorAfter(cursor))
		}
		var out fileListResponse
		path := fmt.Sprintf("/storage/buckets/%s/files", bucketID)
		if err := c.request(ctx, op, http.MethodGet, path, QueryParams(queries...), nil, &out); err != nil {
			return nil, err
		}
		return out.Files, nil
	})
}
