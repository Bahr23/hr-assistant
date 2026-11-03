import { describe, expect, it } from 'vitest'

import type { components } from '@/shared/api'
import { candidateName, formatDuration } from './formatCandidate'

type Candidate = components['schemas']['CandidateProfile']

const baseCandidate: Candidate = {
  telegram_user_id: 455788012,
  username: 'Bahr0o',
  first_name: 'Ivan',
  last_name: 'Petrov',
  is_premium: true,
  language_code: 'ru',
  first_seen_at: '2026-08-31T15:00:00Z',
  last_seen_at: '2026-08-31T15:02:00Z',
}

describe('candidateName', () => {
  it('prefers full name', () => {
    expect(candidateName(baseCandidate)).toBe('Ivan Petrov')
  })

  it('falls back to username and Telegram id', () => {
    expect(candidateName({ ...baseCandidate, first_name: '', last_name: null })).toBe('@Bahr0o')
    expect(candidateName({ ...baseCandidate, first_name: '', last_name: null, username: null })).toBe('Кандидат 455788012')
  })
})

describe('formatDuration', () => {
  it('formats seconds and minutes', () => {
    expect(formatDuration(42)).toBe('42 сек.')
    expect(formatDuration(120)).toBe('2 мин.')
    expect(formatDuration(125)).toBe('2 мин. 5 сек.')
  })
})
