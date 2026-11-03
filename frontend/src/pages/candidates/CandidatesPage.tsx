import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Search, Users } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom'

import { candidateName, candidateUsername, formatDuration } from '@/entities/candidate'
import { candidateListQuery, candidateVacancyOptionsQuery } from '@/features/candidates'
import { Button, SelectField, Spinner, TextField } from '@/shared/ui'
import styles from './CandidatesPage.module.css'

const PAGE_SIZE = 20
const dateFormatter = new Intl.DateTimeFormat('ru-RU', {
  dateStyle: 'medium',
  timeStyle: 'short',
})

function readOffset(value: string | null) {
  const offset = Number(value)
  return Number.isInteger(offset) && offset >= 0 ? offset : 0
}

export function CandidatesPage() {
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const location = useLocation()
  const search = params.get('search')?.trim() ?? ''
  const vacancyId = params.get('vacancy_id') ?? ''
  const offset = readOffset(params.get('offset'))
  const [searchInput, setSearchInput] = useState(search)
  const candidates = useQuery(candidateListQuery({
    search: search || undefined,
    vacancyId: vacancyId || undefined,
    limit: PAGE_SIZE,
    offset,
  }))
  const vacancies = useQuery(candidateVacancyOptionsQuery)

  useEffect(() => {
    const normalized = searchInput.trim()
    if (normalized === search) return
    const timeout = window.setTimeout(() => {
      setParams((current) => {
        const next = new URLSearchParams(current)
        if (normalized) next.set('search', normalized)
        else next.delete('search')
        next.delete('offset')
        return next
      }, { replace: true })
    }, 350)
    return () => window.clearTimeout(timeout)
  }, [search, searchInput, setParams])

  const vacancyOptions = useMemo(() => [
    { value: '', label: 'Все вакансии' },
    ...(vacancies.data ?? []).map((vacancy) => ({ value: vacancy.id, label: vacancy.title || 'Без названия' })),
  ], [vacancies.data])
  const items = candidates.data?.items ?? []
  const total = candidates.data?.total ?? 0
  const from = total === 0 ? 0 : offset + 1
  const to = Math.min(offset + PAGE_SIZE, total)
  const returnTo = `${location.pathname}${location.search}`

  const updateVacancy = (value: string) => {
    setParams((current) => {
      const next = new URLSearchParams(current)
      if (value) next.set('vacancy_id', value)
      else next.delete('vacancy_id')
      next.delete('offset')
      return next
    })
  }

  const updateOffset = (nextOffset: number) => {
    setParams((current) => {
      const next = new URLSearchParams(current)
      if (nextOffset > 0) next.set('offset', String(nextOffset))
      else next.delete('offset')
      return next
    })
  }

  const openCandidate = (sessionId: string) => {
    void navigate(`/candidates/${sessionId}`, { state: { returnTo } })
  }

  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <div>
          <p className={styles.eyebrow}>Завершённые интервью</p>
          <h1>Кандидаты</h1>
          <p>{candidates.isSuccess ? `${total} заявок с завершёнными ответами` : 'Ответы кандидатов по вашим вакансиям.'}</p>
        </div>
      </header>

      <section className={styles.content}>
        <div className={styles.toolbar}>
          <TextField
            className={styles.search}
            label="Поиск кандидата"
            name="candidate-search"
            type="search"
            value={searchInput}
            placeholder="Имя или Telegram username"
            onChange={(event) => setSearchInput(event.target.value)}
          />
          <SelectField
            className={styles.vacancyFilter}
            label="Вакансия"
            value={vacancyId}
            options={vacancyOptions}
            disabled={vacancies.isPending || vacancies.isError}
            onValueChange={updateVacancy}
          />
        </div>

        {candidates.isPending ? <Spinner label="Загружаем кандидатов…" /> : null}
        {candidates.isError ? (
          <div className={styles.empty}>
            <Search aria-hidden="true" />
            <h2>Не удалось загрузить кандидатов</h2>
            <p>Проверьте соединение и повторите запрос.</p>
            <Button onClick={() => void candidates.refetch()}>Повторить</Button>
          </div>
        ) : null}
        {candidates.isSuccess && items.length === 0 ? (
          <div className={styles.empty}>
            <Users aria-hidden="true" />
            <h2>{search || vacancyId ? 'Ничего не найдено' : 'Завершённых интервью пока нет'}</h2>
            <p>{search || vacancyId ? 'Измените поиск или выберите другую вакансию.' : 'Кандидаты появятся здесь после завершения интервью.'}</p>
          </div>
        ) : null}

        {items.length > 0 ? (
          <>
            <div className={styles.tableWrap}>
              <table className={styles.table}>
                <thead>
                  <tr>
                    <th>Кандидат</th>
                    <th>Вакансия</th>
                    <th>Завершено</th>
                    <th>Длительность</th>
                    <th>Ответы</th>
                    <th aria-label="Открыть" />
                  </tr>
                </thead>
                <tbody>
                  {items.map((item) => {
                    const username = candidateUsername(item.candidate)
                    return (
                      <tr
                        key={item.id}
                        role="link"
                        tabIndex={0}
                        aria-label={`Открыть интервью кандидата ${candidateName(item.candidate)}`}
                        onClick={() => openCandidate(item.id)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter' || event.key === ' ') {
                            event.preventDefault()
                            openCandidate(item.id)
                          }
                        }}
                      >
                        <td>
                          <div className={styles.candidateCell}>
                            <span className={styles.avatar}>{candidateName(item.candidate).slice(0, 1).toUpperCase()}</span>
                            <span><strong>{candidateName(item.candidate)}</strong>{username ? <small>{username}</small> : null}</span>
                          </div>
                        </td>
                        <td>{item.vacancy.title || 'Без названия'}</td>
                        <td><time dateTime={item.completed_at}>{dateFormatter.format(new Date(item.completed_at))}</time></td>
                        <td>{formatDuration(item.duration_seconds)}</td>
                        <td>{item.answer_count}</td>
                        <td><ChevronRight className={styles.rowArrow} aria-hidden="true" /></td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
            <footer className={styles.pagination}>
              <span>Показано {from}–{to} из {total}</span>
              <div>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={offset === 0}
                  aria-label="Предыдущая страница"
                  onClick={() => updateOffset(Math.max(0, offset - PAGE_SIZE))}
                ><ChevronLeft size={18} /> Назад</Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={offset + PAGE_SIZE >= total}
                  aria-label="Следующая страница"
                  onClick={() => updateOffset(offset + PAGE_SIZE)}
                >Далее <ChevronRight size={18} /></Button>
              </div>
            </footer>
          </>
        ) : null}
      </section>
    </main>
  )
}
