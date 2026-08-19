package automation_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aiops/aiops-platform/internal/automation"
)

func TestArgoCDListApps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/applications" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"items": [
					{
						"name": "my-app",
						"namespace": "argocd",
						"project": "default",
						"status": {"sync": {"status": "Synced"}, "health": {"status": "Healthy"}},
						"spec": {"source": {"repoURL": "https://github.com/org/repo", "path": "charts/my-app", "targetRevision": "main"}, "destination": {"server": "https://kubernetes.default.svc", "namespace": "default"}}
					},
					{
						"name": "other-app",
						"namespace": "argocd",
						"project": "default",
						"status": {"sync": {"status": "OutOfSync"}, "health": {"status": "Degraded"}},
						"spec": {"source": {"repoURL": "https://github.com/org/repo", "path": "charts/other", "targetRevision": "main"}, "destination": {"server": "https://kubernetes.default.svc", "namespace": "prod"}}
					}
				]
			}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := automation.NewArgoCDClient(server.URL, "test-token", 10)
	apps, err := client.ListApplications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(apps))
	}
	if apps[0].Name != "my-app" {
		t.Fatalf("expected first app my-app, got %s", apps[0].Name)
	}
	if apps[0].Status.Sync.Status != "Synced" {
		t.Fatalf("expected sync status Synced, got %s", apps[0].Status.Sync.Status)
	}
	if apps[1].Status.Health.Status != "Degraded" {
		t.Fatalf("expected health Degraded, got %s", apps[1].Status.Health.Status)
	}
}

func TestArgoCDGetApp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/applications/my-app" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"name": "my-app",
				"namespace": "argocd",
				"project": "default",
				"status": {"sync": {"status": "Synced"}, "health": {"status": "Healthy"}}
			}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := automation.NewArgoCDClient(server.URL, "test-token", 10)
	app, err := client.GetApplication(context.Background(), "my-app")
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "my-app" {
		t.Fatalf("expected app my-app, got %s", app.Name)
	}
}

func TestArgoCDSync(t *testing.T) {
	var syncCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/applications/my-app/sync" && r.Method == "POST" {
			syncCalled = true
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := automation.NewArgoCDClient(server.URL, "test-token", 10)
	err := client.Sync(context.Background(), "my-app")
	if err != nil {
		t.Fatal(err)
	}
	if !syncCalled {
		t.Fatal("expected sync endpoint to be called")
	}
}

func TestArgoCDRefresh(t *testing.T) {
	var refreshCalled bool
	var hardParam string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/applications/my-app/refresh" && r.Method == "POST" {
			refreshCalled = true
			hardParam = r.URL.Query().Get("hard")
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := automation.NewArgoCDClient(server.URL, "test-token", 10)
	err := client.Refresh(context.Background(), "my-app", true)
	if err != nil {
		t.Fatal(err)
	}
	if !refreshCalled {
		t.Fatal("expected refresh endpoint to be called")
	}
	if hardParam != "true" {
		t.Fatalf("expected hard=true, got %s", hardParam)
	}
}

func TestArgoCDNilClient(t *testing.T) {
	var client *automation.ArgoCDClient
	_, err := client.ListApplications(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestArgoCDAuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	client := automation.NewArgoCDClient(server.URL, "secret-token", 10)
	_, _ = client.ListApplications(context.Background())

	if authHeader != "Bearer secret-token" {
		t.Fatalf("expected Bearer secret-token, got %s", authHeader)
	}
}

func TestArgoCDError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error": "Unauthorized"}`))
	}))
	defer server.Close()

	client := automation.NewArgoCDClient(server.URL, "bad-token", 10)
	_, err := client.ListApplications(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
}
