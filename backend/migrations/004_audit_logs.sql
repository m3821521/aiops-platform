-- 审计日志表
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NULL,
    username VARCHAR(64) NULL,
    action VARCHAR(64) NOT NULL,       -- create / update / delete / restart / scale / sync / build / login
    resource VARCHAR(64) NOT NULL,     -- user / pod / deployment / alert / argocd / jenkins
    resource_id VARCHAR(128) NULL,     -- 资源 ID 或名称
    request TEXT NULL,                 -- 请求体 JSON
    result VARCHAR(16) NOT NULL DEFAULT 'success', -- success / failed
    error_message TEXT NULL,
    ip VARCHAR(45) NULL,
    user_agent VARCHAR(255) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_audit_user (user_id),
    INDEX idx_audit_action (action),
    INDEX idx_audit_resource (resource),
    INDEX idx_audit_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
