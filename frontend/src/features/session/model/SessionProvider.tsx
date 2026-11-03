import { type PropsWithChildren, useCallback, useEffect, useMemo, useState } from 'react'

import { sessionApi } from '@/features/session/api/sessionApi'
import type { components } from '@/shared/api'
import { SessionContext, type SessionStatus } from './sessionContext'

type User = components['schemas']['User']
type LoginInput = components['schemas']['LoginRequest']
type RegisterInput = components['schemas']['RegisterRequest']
export function SessionProvider({ children }: PropsWithChildren) {
  const [status, setStatus] = useState<SessionStatus>('bootstrapping')
  const [user, setUser] = useState<User | null>(null)

  useEffect(() => {
    let active = true
    sessionApi.refresh().then(
      (auth) => {
        if (!active) return
        setUser(auth.user)
        setStatus('authenticated')
      },
      () => {
        if (!active) return
        setUser(null)
        setStatus('anonymous')
      },
    )
    return () => {
      active = false
    }
  }, [])

  const login = useCallback(async (input: LoginInput) => {
    const auth = await sessionApi.login(input)
    setUser(auth.user)
    setStatus('authenticated')
  }, [])

  const register = useCallback(async (input: RegisterInput) => {
    const auth = await sessionApi.register(input)
    setUser(auth.user)
    setStatus('authenticated')
  }, [])

  const logout = useCallback(async () => {
    try {
      await sessionApi.logout()
    } finally {
      setUser(null)
      setStatus('anonymous')
    }
  }, [])

  const value = useMemo(() => ({ status, user, login, register, logout }), [status, user, login, register, logout])
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}
