import { ArrowUpRight, BriefcaseBusiness, CalendarDays, Link2 } from 'lucide-react'
import { Link } from 'react-router-dom'

import type { components } from '@/shared/api'
import { VacancyStatusBadge } from './VacancyStatusBadge'
import styles from './VacancyCard.module.css'

type Vacancy = components['schemas']['Vacancy']

function formatDate(value: string) {
  return new Intl.DateTimeFormat('ru', { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(value))
}

export function VacancyCard({ vacancy }: { vacancy: Vacancy }) {
  return (
    <Link className={styles.card} to={`/vacancies/${vacancy.id}`}>
      <div className={styles.top}>
        <VacancyStatusBadge status={vacancy.status} />
        <ArrowUpRight className={styles.arrow} aria-hidden="true" />
      </div>
      <div className={styles.title}>
        <span className={styles.icon}><BriefcaseBusiness aria-hidden="true" /></span>
        <div>
          <h2>{vacancy.title || 'Вакансия без названия'}</h2>
        </div>
      </div>
      <div className={styles.meta}>
        <span><CalendarDays /> Обновлена {formatDate(vacancy.updated_at)}</span>
        {vacancy.interview_url ? <span className={styles.online}><Link2 /> Ссылка активна</span> : null}
      </div>
    </Link>
  )
}
