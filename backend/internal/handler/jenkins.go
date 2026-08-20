package handler

import (
	"strconv"

	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// JenkinsHandler 处理 Jenkins 请求。
type JenkinsHandler struct {
	Jenkins *automation.JenkinsClient
}

// ListJobs 处理 GET /api/v1/jenkins/jobs
func (h *JenkinsHandler) ListJobs(c *gin.Context) {
	if h.Jenkins == nil {
		response.ServiceUnavailable(c, "Jenkins 服务未配置")
		return
	}

	jobs, err := h.Jenkins.ListJobs(c.Request.Context())
	if err != nil {
		response.ServiceUnavailable(c, "Jenkins 服务不可用: "+err.Error())
		return
	}

	response.OK(c, jobs)
}

// ListBuilds 处理 GET /api/v1/jenkins/jobs/:name/builds
func (h *JenkinsHandler) ListBuilds(c *gin.Context) {
	if h.Jenkins == nil {
		response.ServiceUnavailable(c, "Jenkins 服务未配置")
		return
	}

	jobName := c.Param("name")
	builds, err := h.Jenkins.ListBuilds(c.Request.Context(), jobName)
	if err != nil {
		response.ServiceUnavailable(c, "Jenkins 服务不可用: "+err.Error())
		return
	}

	response.OK(c, builds)
}

// Build 处理 POST /api/v1/jenkins/jobs/:name/build
func (h *JenkinsHandler) Build(c *gin.Context) {
	if h.Jenkins == nil {
		response.ServiceUnavailable(c, "Jenkins 服务未配置")
		return
	}

	jobName := c.Param("name")
	if err := h.Jenkins.Build(c.Request.Context(), jobName); err != nil {
		response.ServiceUnavailable(c, "Jenkins 服务不可用: "+err.Error())
		return
	}

	response.OK(c, gin.H{"message": "构建已触发", "job": jobName})
}

// GetBuildLog 处理 GET /api/v1/jenkins/jobs/:name/builds/:number/log
func (h *JenkinsHandler) GetBuildLog(c *gin.Context) {
	if h.Jenkins == nil {
		response.ServiceUnavailable(c, "Jenkins 服务未配置")
		return
	}

	jobName := c.Param("name")
	numberStr := c.Param("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		response.BadRequest(c, "构建编号格式错误")
		return
	}

	log, err := h.Jenkins.GetBuildLog(c.Request.Context(), jobName, number)
	if err != nil {
		response.ServiceUnavailable(c, "Jenkins 服务不可用: "+err.Error())
		return
	}

	response.OK(c, gin.H{"log": log})
}
