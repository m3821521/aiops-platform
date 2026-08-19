package automation_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aiops/aiops-platform/internal/automation"
)

func TestJenkinsListJobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/json" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"jobs": [
					{"name": "build-app", "url": "http://jenkins/job/build-app", "color": "blue", "lastBuild": {"number": 42, "result": "SUCCESS"}},
					{"name": "deploy-prod", "url": "http://jenkins/job/deploy-prod", "color": "red", "lastBuild": {"number": 10, "result": "FAILURE"}}
				]
			}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := automation.NewJenkinsClient(server.URL, "admin", "token", 10)
	jobs, err := client.ListJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].Name != "build-app" {
		t.Fatalf("expected first job build-app, got %s", jobs[0].Name)
	}
	if jobs[0].Color != "blue" {
		t.Fatalf("expected color blue, got %s", jobs[0].Color)
	}
}

func TestJenkinsListBuilds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/job/my-job/api/json" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"builds": [
					{"number": 3, "result": "SUCCESS", "timestamp": 1700000000000, "duration": 120000, "building": false, "url": "http://jenkins/job/my-job/3"},
					{"number": 2, "result": "FAILURE", "timestamp": 1699999000000, "duration": 60000, "building": false, "url": "http://jenkins/job/my-job/2"},
					{"number": 1, "result": null, "timestamp": 1699998000000, "duration": 0, "building": true, "url": "http://jenkins/job/my-job/1"}
				]
			}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := automation.NewJenkinsClient(server.URL, "admin", "token", 10)
	builds, err := client.ListBuilds(context.Background(), "my-job")
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 3 {
		t.Fatalf("expected 3 builds, got %d", len(builds))
	}
	if builds[0].Number != 3 {
		t.Fatalf("expected first build number 3, got %d", builds[0].Number)
	}
	if builds[2].Building != true {
		t.Fatal("expected third build to be building")
	}
}

func TestJenkinsBuild(t *testing.T) {
	var buildCalled bool
	var crumbCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/crumbIssuer/api/json" {
			crumbCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"crumb": "test-crumb"}`))
			return
		}
		if r.URL.Path == "/job/my-job/build" && r.Method == "POST" {
			buildCalled = true
			if r.Header.Get("Jenkins-Crumb") != "test-crumb" {
				t.Error("expected Jenkins-Crumb header")
			}
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := automation.NewJenkinsClient(server.URL, "admin", "token", 10)
	err := client.Build(context.Background(), "my-job")
	if err != nil {
		t.Fatal(err)
	}
	if !crumbCalled {
		t.Fatal("expected crumb request")
	}
	if !buildCalled {
		t.Fatal("expected build request")
	}
}

func TestJenkinsGetBuildLog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/job/my-job/42/consoleText" {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("Building in workspace\n[INFO] Compiling...\n[SUCCESS] Build complete\n"))
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	client := automation.NewJenkinsClient(server.URL, "admin", "token", 10)
	log, err := client.GetBuildLog(context.Background(), "my-job", 42)
	if err != nil {
		t.Fatal(err)
	}
	if log == "" {
		t.Fatal("expected non-empty log")
	}
	if !contains(log, "Build complete") {
		t.Fatalf("expected log to contain 'Build complete', got %s", log)
	}
}

func TestJenkinsNilClient(t *testing.T) {
	var client *automation.JenkinsClient
	_, err := client.ListJobs(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestJenkinsAuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jobs":[]}`))
	}))
	defer server.Close()

	client := automation.NewJenkinsClient(server.URL, "admin", "secret-token", 10)
	_, _ = client.ListJobs(context.Background())

	if authHeader == "" {
		t.Fatal("expected Authorization header")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
