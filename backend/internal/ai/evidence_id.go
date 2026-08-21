package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateEvidenceID 生成 deterministic Evidence ID。
// 格式：E-<sha256前8字符>
// 输入：incident_id + source + type + resource + timestamp + content
// 同一次 Collection 中，同一 Evidence 得到相同 ID。
func GenerateEvidenceID(incidentID int64, source, evType, resource string, timestamp time.Time, content string) string {
	raw := fmt.Sprintf("%d|%s|%s|%s|%s|%s",
		incidentID,
		source,
		evType,
		resource,
		timestamp.UTC().Format(time.RFC3339),
		content,
	)
	hash := sha256.Sum256([]byte(raw))
	return "E-" + hex.EncodeToString(hash[:])[:8]
}

// CollectEvidenceIDs 从 AIContext 中收集所有 Evidence ID，用于验证 AI 引用。
func CollectEvidenceIDs(ctx AIContext) map[string]bool {
	ids := make(map[string]bool)
	for _, a := range ctx.Alerts {
		if a.ID != "" {
			ids[a.ID] = true
		}
	}
	for _, a := range ctx.Anomalies {
		if a.ID != "" {
			ids[a.ID] = true
		}
	}
	for _, m := range ctx.Metrics {
		if m.ID != "" {
			ids[m.ID] = true
		}
	}
	for _, l := range ctx.Logs {
		if l.ID != "" {
			ids[l.ID] = true
		}
	}
	for _, e := range ctx.Events {
		if e.ID != "" {
			ids[e.ID] = true
		}
	}
	if ctx.PodDiagnostic != nil && ctx.PodDiagnostic.ID != "" {
		ids[ctx.PodDiagnostic.ID] = true
	}
	if ctx.DeploymentDiagnostic != nil && ctx.DeploymentDiagnostic.ID != "" {
		ids[ctx.DeploymentDiagnostic.ID] = true
	}
	if ctx.ServiceDiagnostic != nil && ctx.ServiceDiagnostic.ID != "" {
		ids[ctx.ServiceDiagnostic.ID] = true
	}
	return ids
}

// ValidateEvidenceReferences 验证 AI 输出的 Evidence References 是否都存在于 Context 中。
// 返回 accepted（有效引用）和 rejected（无效引用的 ID 列表）。
func ValidateEvidenceReferences(result *AIAnalysisResult, validIDs map[string]bool) (accepted []AIEvidence, rejected []string) {
	if result == nil {
		return nil, nil
	}
	for _, e := range result.Evidence {
		if e.ID == "" {
			rejected = append(rejected, "(empty id)")
			continue
		}
		if validIDs[e.ID] {
			accepted = append(accepted, e)
		} else {
			rejected = append(rejected, e.ID)
		}
	}
	return accepted, rejected
}
