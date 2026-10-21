package chatusage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleUpdateUsageUsesConfiguredBaseURL(t *testing.T) {
	userID := "user-1"
	var gotPut bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"users": []map[string]string{{"id": userID, "balance": "2.5"}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/users/"+userID+"/balance":
			gotPut = true
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	err := HandleUpdateUsage(context.Background(), server.URL+"/api/v1", "token", Event{
		UserID: userID,
		Amount: 10000,
		Status: "completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gotPut {
		t.Fatal("configured base URL did not receive balance update")
	}
}
