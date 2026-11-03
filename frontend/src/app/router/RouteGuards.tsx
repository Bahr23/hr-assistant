import { Navigate, Outlet, useLocation } from 'react-router-dom'

import { useSession } from '@/features/session'
import { Spinner } from '@/shared/ui'

export function ProtectedRoute() {
  const session = useSession()
  const location = useLocation()

  if (session.status === 'bootstrapping') return <Spinner label="Открываем рабочее пространство…" />
  if (session.status === 'anonymous') return <Navigate to="/login" state={{ from: location }} replace />
  return <Outlet />
}

export function AnonymousRoute() {
  const session = useSession()
  if (session.status === 'bootstrapping') return <Spinner label="Проверяем сессию…" />
  if (session.status === 'authenticated') return <Navigate to="/vacancies" replace />
  return <Outlet />
}
