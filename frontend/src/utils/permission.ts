import { useAuthStore } from '@/stores/auth'

/**
 * 检查当前用户是否拥有指定权限。
 * admin 角色拥有所有权限。
 */
export function hasPermission(resource: string, action: string): boolean {
  const user = useAuthStore.getState().user
  if (!user || !user.roles) return false

  const required = `${resource}:${action}`
  for (const role of user.roles) {
    if (role.name === 'admin') return true
    if (role.permissions) {
      for (const perm of role.permissions) {
        if (perm.name === required) return true
      }
    }
  }
  return false
}

/**
 * 检查当前用户是否为指定角色。
 */
export function hasRole(roleName: string): boolean {
  const user = useAuthStore.getState().user
  if (!user || !user.roles) return false
  return user.roles.some((r) => r.name === roleName)
}

/**
 * React Hook 版本：在组件中使用，响应式更新。
 */
export function usePermission(resource: string, action: string): boolean {
  const user = useAuthStore((s) => s.user)
  if (!user || !user.roles) return false

  const required = `${resource}:${action}`
  for (const role of user.roles) {
    if (role.name === 'admin') return true
    if (role.permissions) {
      for (const perm of role.permissions) {
        if (perm.name === required) return true
      }
    }
  }
  return false
}
