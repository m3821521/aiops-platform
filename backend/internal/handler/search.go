package handler

import (
	"strconv"
	"strings"

	"github.com/aiops/aiops-platform/internal/alert"
	"github.com/aiops/aiops-platform/internal/cluster"
	"github.com/aiops/aiops-platform/internal/incident"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// SearchHandler 处理全局搜索。
type SearchHandler struct {
	IncidentRepo *incident.Repository
	AlertRepo    *alert.Repository
	ClusterSvc   *cluster.Service
}

// SearchResult 表示一条搜索结果。
type SearchResult struct {
	Type        string `json:"type"`        // incident / alert / pod / node / deployment
	ID          string `json:"id"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	Severity    string `json:"severity,omitempty"`
	Status      string `json:"status,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	URL         string `json:"url"`
}

// SearchResponse 是搜索响应。
type SearchResponse struct {
	Query   string         `json:"query"`
	Total   int            `json:"total"`
	Results []SearchResult `json:"results"`
}

// Search 处理 GET /api/v1/search?q=xxx
func (h *SearchHandler) Search(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(200, SearchResponse{Query: "", Total: 0, Results: []SearchResult{}})
		return
	}

	results := make([]SearchResult, 0, 50)
	queryLower := strings.ToLower(query)

	// 搜索 Incidents
	if h.IncidentRepo != nil {
		incidents, _, err := h.IncidentRepo.List(c.Request.Context(), incident.ListFilter{}, 1, 20)
		if err == nil {
			for _, inc := range incidents {
				if strings.Contains(strings.ToLower(inc.Title), queryLower) ||
					strings.Contains(strings.ToLower(inc.Service), queryLower) {
					results = append(results, SearchResult{
						Type:     "incident",
						ID:       int64ToString(inc.ID),
						Title:    inc.Title,
						Subtitle: inc.Service,
						Severity: inc.Severity,
						Status:   inc.Status,
						URL:      "/incidents/" + int64ToString(inc.ID),
					})
				}
			}
		}
	}

	// 搜索 Alerts
	if h.AlertRepo != nil {
		alerts, _, err := h.AlertRepo.List(c.Request.Context(), alert.ListFilter{}, 1, 30)
		if err == nil {
			for _, a := range alerts {
				if strings.Contains(strings.ToLower(a.Labels["alertname"]), queryLower) ||
					strings.Contains(strings.ToLower(a.Labels["namespace"]), queryLower) ||
					strings.Contains(strings.ToLower(a.Labels["pod"]), queryLower) {
					name := a.Labels["alertname"]
					if name == "" {
						name = "Alert"
					}
					results = append(results, SearchResult{
						Type:      "alert",
						ID:        int64ToString(a.ID),
						Title:     name,
						Subtitle:  a.Labels["namespace"] + "/" + a.Labels["pod"],
						Severity:  a.Labels["severity"],
						Status:    a.Status,
						Namespace: a.Labels["namespace"],
						URL:       "/alerts",
					})
				}
			}
		}
	}

	// 搜索 Pods
	if h.ClusterSvc != nil {
		pods, err := h.ClusterSvc.ListPods(c.Request.Context(), "", "")
		if err == nil {
			for _, p := range pods {
				if strings.Contains(strings.ToLower(p.Name), queryLower) ||
					strings.Contains(strings.ToLower(p.Namespace), queryLower) {
					results = append(results, SearchResult{
						Type:      "pod",
						ID:        p.Name,
						Title:     p.Name,
						Subtitle:  p.Namespace + " / " + p.Spec.NodeName,
						Status:    string(p.Status.Phase),
						Namespace: p.Namespace,
						URL:       "/kubernetes/pods/" + p.Name + "?namespace=" + p.Namespace,
					})
				}
			}
		}
	}

	response.OK(c, SearchResponse{
		Query:   query,
		Total:   len(results),
		Results: results,
	})
}

func int64ToString(n int64) string {
	return strconv.FormatInt(n, 10)
}
