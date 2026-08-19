-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    email VARCHAR(128),
    full_name VARCHAR(128),
    status VARCHAR(16) NOT NULL DEFAULT 'active', -- active / disabled
    last_login_at DATETIME NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL,
    INDEX idx_users_username (username),
    INDEX idx_users_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 角色表
CREATE TABLE IF NOT EXISTS roles (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(64) NOT NULL UNIQUE, -- admin / operator / viewer
    description VARCHAR(255),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_roles_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 权限表
CREATE TABLE IF NOT EXISTS permissions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE, -- 如 alerts:read, pods:write
    resource VARCHAR(64) NOT NULL,     -- 如 alerts, pods, deployments
    action VARCHAR(16) NOT NULL,       -- read / write
    description VARCHAR(255),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_permissions_name (name),
    INDEX idx_permissions_resource (resource)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 用户-角色关联表
CREATE TABLE IF NOT EXISTS user_roles (
    user_id BIGINT NOT NULL,
    role_id BIGINT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, role_id),
    INDEX idx_user_roles_user (user_id),
    INDEX idx_user_roles_role (role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 角色-权限关联表
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id BIGINT NOT NULL,
    permission_id BIGINT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (role_id, permission_id),
    INDEX idx_role_permissions_role (role_id),
    INDEX idx_role_permissions_permission (permission_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 初始化默认角色
INSERT INTO roles (name, description) VALUES
    ('admin', '管理员，拥有所有权限'),
    ('operator', '运维人员，可执行运维操作'),
    ('viewer', '只读用户，只能查看数据')
ON DUPLICATE KEY UPDATE description=VALUES(description);

-- 初始化默认权限
INSERT INTO permissions (name, resource, action, description) VALUES
    ('alerts:read', 'alerts', 'read', '查看告警'),
    ('alerts:write', 'alerts', 'write', '确认/关闭告警'),
    ('metrics:read', 'metrics', 'read', '查看指标'),
    ('logs:read', 'logs', 'read', '查看日志'),
    ('cluster:read', 'cluster', 'read', '查看集群资源'),
    ('cluster:write', 'cluster', 'write', '操作集群资源（重启/扩容）'),
    ('jenkins:read', 'jenkins', 'read', '查看 Jenkins 构建'),
    ('jenkins:write', 'jenkins', 'write', '触发 Jenkins 构建'),
    ('argocd:read', 'argocd', 'read', '查看 ArgoCD 应用'),
    ('argocd:write', 'argocd', 'write', 'Sync/Refresh ArgoCD 应用'),
    ('users:read', 'users', 'read', '查看用户'),
    ('users:write', 'users', 'write', '管理用户')
ON DUPLICATE KEY UPDATE description=VALUES(description);

-- admin 角色拥有所有权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p WHERE r.name = 'admin'
ON DUPLICATE KEY UPDATE role_id=role_id;

-- operator 角色拥有读写权限（除用户管理）
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'operator' AND p.name NOT LIKE 'users:%'
ON DUPLICATE KEY UPDATE role_id=role_id;

-- viewer 角色只有只读权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name = 'viewer' AND p.action = 'read'
ON DUPLICATE KEY UPDATE role_id=role_id;

-- 默认管理员用户（密码: admin123，bcrypt hash）
-- 生产环境请立即修改密码
INSERT INTO users (username, password_hash, email, full_name, status) VALUES
    ('admin', '$2a$10$kJOvYGMzZmnNBpno2Fa3cuxkIy5.bhmOPwz1Uvh/PLmmdPTT0W5Yy', 'admin@example.com', 'System Admin', 'active')
ON DUPLICATE KEY UPDATE username=username;

-- 给 admin 用户分配 admin 角色
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u, roles r WHERE u.username = 'admin' AND r.name = 'admin'
ON DUPLICATE KEY UPDATE user_id=user_id;
