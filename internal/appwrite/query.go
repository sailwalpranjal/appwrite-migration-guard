package appwrite

import (
	"encoding/json"
	"net/url"
)

// Query builds Appwrite "queries[]" filter/pagination strings. The wire
// format ({"method":...,"attribute":...,"values":[...]}) is verified
// against utopia-php/database 7.3.11 (Query::toArray), the query engine
// Appwrite 2.3.0 depends on — not guessed.

type rawQuery struct {
	Method    string `json:"method"`
	Attribute string `json:"attribute,omitempty"`
	Values    []any  `json:"values,omitempty"`
}

func encode(q rawQuery) string {
	b, err := json.Marshal(q)
	if err != nil {
		// rawQuery only ever holds JSON-marshalable primitives; this cannot fail.
		panic(err)
	}
	return string(b)
}

// Limit returns a "limit" pagination query.
func Limit(n int) string {
	return encode(rawQuery{Method: "limit", Values: []any{n}})
}

// CursorAfter returns a "cursorAfter" pagination query for the given
// document/resource ID.
func CursorAfter(id string) string {
	return encode(rawQuery{Method: "cursorAfter", Values: []any{id}})
}

// OrderAsc returns an "orderAsc" query for attribute, used to make listing
// order deterministic across pages.
func OrderAsc(attribute string) string {
	return encode(rawQuery{Method: "orderAsc", Attribute: attribute, Values: []any{}})
}

// Equal returns an "equal" filter query.
func Equal(attribute string, value any) string {
	return encode(rawQuery{Method: "equal", Attribute: attribute, Values: []any{value}})
}

// QueryParams builds a url.Values with a repeated "queries[]" parameter,
// as expected by Appwrite's REST list endpoints.
func QueryParams(queries ...string) url.Values {
	v := url.Values{}
	for _, q := range queries {
		v.Add("queries[]", q)
	}
	return v
}
