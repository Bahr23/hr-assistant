import { useQuery } from '@tanstack/react-query'
import { BriefcaseBusiness, Plus, Search } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router-dom'

import { VacancyCard } from '@/entities/vacancy'
import { vacancyListQuery } from '@/features/vacancies'
import type { components } from '@/shared/api'
import { Button, Spinner } from '@/shared/ui'
import styles from './VacanciesPage.module.css'

type Status = components['schemas']['VacancyStatus']
const filters: Array<{ value?: Status; label: string }> = [
  { value: 'published', label: 'Активные' },
  { value: 'archived', label: 'Архив' },
]

export function VacanciesPage() {
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const rawStatus = params.get('status')
  const status: Status = rawStatus === 'archived' ? 'archived' : 'published'
  const vacancies = useQuery(vacancyListQuery(status))

  const items = vacancies.data?.items ?? []

  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <div>
          <p className={styles.eyebrow}>Рабочее пространство</p>
          <h1>Вакансии</h1>
          <p>Настраивайте интервью и управляйте активными ссылками.</p>
        </div>
        <Button variant="brand" onClick={() => void navigate('/vacancies/new')}>
          <Plus size={20} /> Новая вакансия
        </Button>
      </header>

      <section className={styles.listSection}>
        <div className={styles.toolbar}>
          <div className={styles.filters} aria-label="Фильтр вакансий">
            {filters.map((filter) => (
              <button
                key={filter.label}
                className={status === filter.value ? styles.selected : undefined}
                onClick={() => setParams(filter.value ? { status: filter.value } : {})}
              >
                {filter.label}
              </button>
            ))}
          </div>
        </div>

        {vacancies.isPending ? <Spinner label="Загружаем вакансии…" /> : null}
        {vacancies.isError ? (
          <div className={styles.empty}><Search /><h2>Не удалось загрузить список</h2><p>Проверьте, запущен ли backend, и попробуйте ещё раз.</p><Button onClick={() => void vacancies.refetch()}>Повторить</Button></div>
        ) : null}
        {vacancies.isSuccess && items.length === 0 ? (
          <div className={styles.empty}><BriefcaseBusiness /><h2>{status === 'archived' ? 'Архив пуст' : 'Создайте первую вакансию'}</h2><p>{status === 'archived' ? 'Архивированные вакансии появятся здесь.' : 'Заполните информацию и вопросы — вакансия появится после завершения всех шагов.'}</p>{status === 'published' ? <Button variant="brand" onClick={() => void navigate('/vacancies/new')}><Plus size={18} /> Создать вакансию</Button> : null}</div>
        ) : null}
        {items.length > 0 ? <div className={styles.grid}>{items.map((vacancy) => <VacancyCard key={vacancy.id} vacancy={vacancy} />)}</div> : null}
      </section>
    </main>
  )
}
