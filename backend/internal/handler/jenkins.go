package handler

import (
	"strconv"

	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/pkg/response"
	"github.com/gin-gonic/gin"
)

// JenkinsHandler 处理 Jenkins 请求。
type JenkinsHandler struct {
	Jenkins  *automation.JenkinsClient
	Resolver automation.JenkinsClientResolver
}

// getClient 根据请求中的 connection_id 参数获取正确的 JenkinsClient。
func (h *JenkinsHandler) getClient(c *gin.Context) (*automation.JenkinsClient, error) {
	if connIDStr := c.Query("connection_id"); connIDStr != "" {
		connID, err := strconv.ParseInt(connIDStr, 10, 64)
		if err != nil {
			return nil, err
		}
		if h.Resolver == nil {
			return nil, nil
		}
		return h.Resolver.BuildJenkinsClientByID(c.Request.Context(), connID)
	}
	return h.Jenkins, nil
}

// ListJobs 处理 GET /api/v1/jenkins/jobs
func (h *JenkinsHandler) ListJobs(c *gin.Context) {
	client, err := h.getClient(c)
	if err != nil {
		response.BadRequest(c, "获取 Jenkins Connection 失败: "+err.Error())
		return
	}
	if client == nil {
		response.ServiceUnavailable(c, "Jenkins 服务未配置")
		return
	}

	jobs, err := client.ListJobs(c.Request.Context())
	if err != nil {
		response.ServiceUnavailable(c, "Jenkins 服务不可用: "+err.Error())
		return
	}

	response.OK(c, jobs)
}

// ListBuilds 处理 GET /api/v1/jenkins/jobs/:name/builds
func (h *JenkinsHandler) ListBuilds(c *gin.Context) {
	client, err := h.getClient(c)
	if err != nil {
		response.BadRequest(c, "获取 Jenkins Connection 失败: "+err.Error())
		return
	}
	if client == nil {
		response.ServiceUnavailable(c, "Jenkins 服务未配置")
		return
	}

	jobName := c.Param("name")
	builds, err := client.ListBuilds(c.Request.Context(), jobName)
	if err != nil {
		response.ServiceUnavailable(c, "Jenkins 服务不可用: "+err.Error())
		return
	}

	response.OK(c, builds)
}

// Build 处理 POST /api/v1/jenkins/jobs/:name/build
func (h *JenkinsHandler) Build(c *gin.Context) {
	client, err := h.getClient(c)
	if err != nil {
		response.BadRequest(c, "获取 Jenkins Connection 失败: "+err.Error())
		return
	}
	if client == nil {
		response.ServiceUnavailable(c, "Jenkins 服务未配置")
		return
	}

	jobName := c.Param("name")
	queueURL, err := client.Build(c.Request.Context(), jobName)
	if err != nil {
		response.ServiceUnavailable(c, "Jenkins 服务不可用: "+err.Error())
		return
	}

	response.OK(c, gin.H{"message": "构建已触发", "job": jobName, "queue_url": queueURL})
}

// GetBuildLog 处理 GET /api/v1/jenkins/jobs/:name/builds/:number/log
func (h *JenkinsHandler) GetBuildLog(c *gin.Context) {
	client, err := h.getClient(c)
	if err != nil {
		response.BadRequest(c, "获取 Jenkins Connection 失败: "+err.Error())
		return
	}
	if client == nil {
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

	log, err := client.GetBuildLog(c.Request.Context(), jobName, number)
	if err != nil {
		response.ServiceUnavailable(c, "Jenkins 服务不可用: "+err.Error())
		return
	}

	response.OK(c, gin.H{"log": log})
}
