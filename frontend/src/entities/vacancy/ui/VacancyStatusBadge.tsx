import type { components } from '@/shared/api'
import styles from './VacancyStatusBadge.module.css'

type Status = components['schemas']['VacancyStatus']

const labels: Record<Status, string> = {
  published: 'Активна',
  archived: 'Архив',
}

export function VacancyStatusBadge({ status }: { status: Status }) {
  return <span className={styles[status]}>{labels[status]}</span>
}
