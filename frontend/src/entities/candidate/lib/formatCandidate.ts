import type { components } from '@/shared/api'

type Candidate = components['schemas']['CandidateProfile']

export function candidateName(candidate: Candidate) {
  const fullName = [candidate.first_name, candidate.last_name].filter(Boolean).join(' ').trim()
  if (fullName) return fullName
  if (candidate.username) return `@${candidate.username}`
  return `Кандидат ${candidate.telegram_user_id}`
}

export function candidateUsername(candidate: Candidate) {
  if (!candidate.username) return null
  return `@${candidate.username}`
}

export function formatDuration(totalSeconds: number) {
  const safeSeconds = Math.max(0, Math.round(totalSeconds))
  const minutes = Math.floor(safeSeconds / 60)
  const seconds = safeSeconds % 60
  if (minutes === 0) return `${seconds} сек.`
  if (seconds === 0) return `${minutes} мин.`
  return `${minutes} мин. ${seconds} сек.`
}
