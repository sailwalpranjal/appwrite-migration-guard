package appwrite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func rawCol(t *testing.T, obj map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return json.RawMessage(b)
}

func TestSchemaDigest_DeterministicRegardlessOfOrderAndTransientFields(t *testing.T) {
	cols1 := []json.RawMessage{
		rawCol(t, map[string]any{"key": "name", "type": "string", "size": 128, "required": true, "$createdAt": "2020-01-01"}),
		rawCol(t, map[string]any{"key": "age", "type": "integer", "required": false, "$updatedAt": "2020-01-01"}),
	}
	// Same columns, reversed order, different transient timestamps/status.
	cols2 := []json.RawMessage{
		rawCol(t, map[string]any{"key": "age", "type": "integer", "required": false, "$updatedAt": "2026-09-27", "status": "available"}),
		rawCol(t, map[string]any{"key": "name", "type": "string", "size": 128, "required": true, "$createdAt": "2026-09-27", "error": ""}),
	}

	d1, err := schemaDigest(cols1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d2, err := schemaDigest(cols2, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("expected identical digests for logically-identical schemas, got %s vs %s", d1, d2)
	}
}

func TestSchemaDigest_ChangesWithRealSchemaDrift(t *testing.T) {
	base := []json.RawMessage{
		rawCol(t, map[string]any{"key": "email", "type": "string", "size": 255, "required": true}),
	}
	changedSize := []json.RawMessage{
		rawCol(t, map[string]any{"key": "email", "type": "string", "size": 20, "required": true}),
	}
	changedRequired := []json.RawMessage{
		rawCol(t, map[string]any{"key": "email", "type": "string", "size": 255, "required": false}),
	}

	dBase, _ := schemaDigest(base, nil)
	dSize, _ := schemaDigest(changedSize, nil)
	dRequired, _ := schemaDigest(changedRequired, nil)

	if dBase == dSize {
		t.Fatal("expected digest to change when column size changes")
	}
	if dBase == dRequired {
		t.Fatal("expected digest to change when column required flag changes")
	}
}

func TestSchemaDigest_ChangesWithIndexDrift(t *testing.T) {
	cols := []json.RawMessage{rawCol(t, map[string]any{"key": "email", "type": "string"})}
	idx1 := []json.RawMessage{rawCol(t, map[string]any{"key": "idx_email", "type": "unique", "columns": []string{"email"}})}
	idx2 := []json.RawMessage{} // index removed

	d1, _ := schemaDigest(cols, idx1)
	d2, _ := schemaDigest(cols, idx2)
	if d1 == d2 {
		t.Fatal("expected digest to change when an index is removed")
	}
}

func TestSchemaDigest_EmptySchemaIsStable(t *testing.T) {
	d1, err := schemaDigest(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d2, err := schemaDigest([]json.RawMessage{}, []json.RawMessage{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("expected nil and empty-slice inputs to produce the same digest, got %s vs %s", d1, d2)
	}
}

// TestListTables_ComputesSchemaDigest is an integration-level check that
// ListTables actually wires schemaDigest in, using real Appwrite-shaped
// column/index JSON from a table's list response.
func TestListTables_ComputesSchemaDigest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total":1,"tables":[{
			"$id":"t1","databaseId":"db1","name":"Widgets",
			"columns":[{"key":"name","type":"string","size":128,"required":true,"status":"available","error":"","$createdAt":"x","$updatedAt":"y"}],
			"indexes":[{"$id":"idx1","key":"idx_name","type":"key","columns":["name"],"status":"available"}]
		}]}`))
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	tables, err := c.ListTables(context.Background(), "db1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].SchemaDigest == "" {
		t.Fatal("expected a non-empty SchemaDigest")
	}
	if tables[0].ColumnCount != 1 || tables[0].IndexCount != 1 {
		t.Fatalf("expected ColumnCount=1 IndexCount=1, got %d/%d", tables[0].ColumnCount, tables[0].IndexCount)
	}
}
