import { Navigate, useLocation } from 'react-router-dom'
import { useAuthStore } from '@/stores/auth'
import { ReactNode, useEffect } from 'react'

interface Props {
  children: ReactNode
}

export default function RequireAuth({ children }: Props) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const fetchMe = useAuthStore((s) => s.fetchMe)
  const token = useAuthStore((s) => s.token)
  const location = useLocation()

  // 有 token 但没有 user 信息时，拉取用户信息
  useEffect(() => {
    if (token && isAuthenticated) {
      fetchMe()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <>{children}</>
}
