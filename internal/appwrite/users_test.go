package appwrite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListUsers_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(userListResponse{
			Total: 2,
			Users: []User{
				{ID: "u1", Name: "Alice", Status: true},
				{ID: "u2", Name: "Bob", Status: false, Labels: []string{"vip"}},
			},
		})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	users, err := c.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[1].Labels[0] != "vip" {
		t.Fatalf("unexpected labels: %v", users[1].Labels)
	}
}

// Regression guard: even if a real Appwrite server includes
// password/hash/email/phone/prefs in the raw JSON (some do, depending on
// scope), User must never surface them — there is no field to decode
// them into, so they are silently dropped by json.Unmarshal.
func TestListUsers_NeverDecodesSensitiveFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"total":1,"users":[{
			"$id":"u1","name":"Alice","status":true,
			"password":"should-never-appear","hash":"argon2","hashOptions":{"type":"argon2"},
			"email":"alice@example.com","phone":"+15551234567",
			"prefs":{"secretToken":"should-also-never-appear"}
		}]}`))
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	users, err := c.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}

	// Marshal the decoded User back to JSON and confirm none of the
	// sensitive raw values survived the round trip.
	b, err := json.Marshal(users[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	for _, forbidden := range []string{"should-never-appear", "argon2", "alice@example.com", "+15551234567", "should-also-never-appear"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("sensitive value %q leaked into decoded User: %s", forbidden, out)
		}
	}
}

func TestListUsers_MultiplePages(t *testing.T) {
	total := pageSize + 3
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		queries := mustDecodeQueries(t, r)
		cursor := ""
		for _, q := range queries {
			if hasQueryMethod([]string{q}, "cursorAfter") {
				var parsed struct {
					Values []string `json:"values"`
				}
				json.Unmarshal([]byte(q), &parsed)
				cursor = parsed.Values[0]
			}
		}
		start := 0
		if cursor != "" {
			for i := 0; i < total; i++ {
				if idForUser(i) == cursor {
					start = i + 1
					break
				}
			}
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		users := make([]User, 0, end-start)
		for i := start; i < end; i++ {
			users = append(users, User{ID: idForUser(i)})
		}
		json.NewEncoder(w).Encode(userListResponse{Total: total, Users: users})
	}))
	defer srv.Close()

	c := New(testEnv(srv.URL))
	users, err := c.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != total {
		t.Fatalf("expected %d users, got %d", total, len(users))
	}
	if requests != 2 {
		t.Fatalf("expected 2 page requests, got %d", requests)
	}
}

func idForUser(i int) string {
	return fmt.Sprintf("u%04d", i)
}
