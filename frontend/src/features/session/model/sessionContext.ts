import { createContext, useContext } from 'react'

import type { components } from '@/shared/api'

type User = components['schemas']['User']
type LoginInput = components['schemas']['LoginRequest']
type RegisterInput = components['schemas']['RegisterRequest']
export type SessionStatus = 'bootstrapping' | 'authenticated' | 'anonymous'

export interface SessionValue {
  status: SessionStatus
  user: User | null
  login(input: LoginInput): Promise<void>
  register(input: RegisterInput): Promise<void>
  logout(): Promise<void>
}

export const SessionContext = createContext<SessionValue | null>(null)

export function useSession() {
  const session = useContext(SessionContext)
  if (!session) throw new Error('useSession must be used inside SessionProvider')
  return session
}
