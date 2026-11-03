import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Check, ChevronDown, FileText, Mic, UserRound } from 'lucide-react'
import { useEffect, useState, type MouseEvent } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'

import { candidateName, candidateUsername, formatDuration } from '@/entities/candidate'
import { CandidateReport, AnswerEvaluation, candidateDetailQuery, candidateKeys, candidatesApi } from '@/features/candidates'
import { ApiError, type components } from '@/shared/api'
import { Button, Spinner } from '@/shared/ui'
import { ReportNavigation } from './ReportNavigation'
import styles from './CandidateDetailPage.module.css'

const dateFormatter = new Intl.DateTimeFormat('ru-RU', { dateStyle: 'long', timeStyle: 'short' })

function safeReturnTo(state: unknown) {
  if (!state || typeof state !== 'object' || !('returnTo' in state)) return '/candidates'
  const returnTo = (state as { returnTo?: unknown }).returnTo
  return typeof returnTo === 'string' && (returnTo === '/candidates' || returnTo.startsWith('/candidates?'))
    ? returnTo : '/candidates'
}

// Native hash navigation must also reveal content inside a closed disclosure.
function revealReportTarget(hash: string) {
  if (hash.startsWith('#report-quote-')) {
    const quote = document.getElementById(hash.slice(1))
    if (!quote) return
    const details = quote.closest('details')
    if (details) details.open = true
    quote.focus({ preventScroll: true })
    quote.scrollIntoView({ block: 'center' })
    return
  }
  if (!/^#answer-\d+$/.test(hash)) return
  const answer = document.getElementById(hash.slice(1))
  if (!(answer instanceof HTMLDetailsElement)) return
  answer.open = true
  answer.querySelector('summary')?.focus({ preventScroll: true })
  answer.scrollIntoView({ block: 'start' })
}

export function CandidateDetailPage() {
  const { sessionId = '' } = useParams()
  const location = useLocation()
  const queryClient = useQueryClient()
  const interview = useQuery(candidateDetailQuery(sessionId))
  const rebuild = useMutation({
    mutationFn: ({ id, version }: { id: string; version: number }) => candidatesApi.rebuildReport(id, version),
    onSuccess: async ({ version }, { id }) => {
      // The accepted version is authoritative. Keep the last result while polling it.
      queryClient.setQueryData<components['schemas']['CandidateInterviewDetail']>(candidateKeys.detail(id), (current) => {
        if (!current || current.report.version >= version) return current
        return { ...current, report: { ...current.report, version, status: 'pending', error_code: null } }
      })
      await queryClient.invalidateQueries({ queryKey: candidateKeys.detail(id) })
    },
  })
  const returnTo = safeReturnTo(location.state)
  const loaded = Boolean(interview.data)
  const [quoteReturn, setQuoteReturn] = useState<{ sessionId: string; answerHash: string; quoteHash: string } | null>(null)

  useEffect(() => {
    const reveal = () => revealReportTarget(window.location.hash)
    reveal()
    window.addEventListener('hashchange', reveal)
    return () => window.removeEventListener('hashchange', reveal)
  }, [loaded, sessionId, location.hash])

  function handleEvidenceClick(event: MouseEvent<HTMLElement>) {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    const anchor = event.target instanceof Element ? event.target.closest('a') : null
    const hash = anchor?.getAttribute('href')
    if (hash && anchor?.id.startsWith('report-quote-')) {
      setQuoteReturn({ sessionId, answerHash: hash, quoteHash: `#${anchor.id}` })
    }
    // Also handle a repeated click when the hash has not changed.
    if (hash) revealReportTarget(hash)
  }

  if (interview.isPending) return <main className={styles.page}><Spinner label="Открываем интервью…" /></main>
  if (!interview.data) {
    const notFound = interview.error instanceof ApiError && interview.error.status === 404
    return <main className={styles.page}>
      <Link className={styles.back} to={returnTo}><ArrowLeft size={18} aria-hidden="true" />К кандидатам</Link>
      <div className={styles.error}>
        <h1>{notFound ? 'Интервью не найдено' : 'Не удалось загрузить интервью'}</h1>
        <p>{notFound ? 'Возможно, оно принадлежит другому пользователю или ещё не завершено.' : 'Проверьте соединение и попробуйте ещё раз.'}</p>
        {!notFound ? <Button onClick={() => void interview.refetch()}>Повторить</Button> : null}
      </div>
    </main>
  }

  const data = interview.data
  const username = candidateUsername(data.candidate)
  const result = data.report?.result
  return <main className={styles.page} onClick={handleEvidenceClick}>
    <div className={styles.breadcrumb}><Link className={styles.back} to={returnTo}><ArrowLeft size={16} aria-hidden="true" />К кандидатам</Link><span aria-hidden="true">/</span><span>Отчёт по интервью</span></div>
    <header className={styles.header}>
      <div className={styles.headerTop}>
        <div className={styles.identity}>
          <div className={styles.avatar} aria-hidden="true">{candidateName(data.candidate).slice(0, 1).toUpperCase()}</div>
          <div className={styles.identityText}>
            <h1>{candidateName(data.candidate)}</h1>
            {username ? <p className={styles.username}><a href={`https://t.me/${encodeURIComponent(username.slice(1))}`} target="_blank" rel="noopener noreferrer">{username}</a></p> : null}
          </div>
        </div>
        <span className={styles.status}><Check size={14} aria-hidden="true" />Интервью завершено</span>
      </div>
      <dl className={styles.interviewDetails}>
        <div><dt>Вакансия</dt><dd><Link to={`/vacancies/${data.vacancy.id}`}>{data.vacancy.title || 'Без названия'}</Link></dd></div>
        <div><dt>Длительность</dt><dd>{formatDuration(data.duration_seconds)}</dd></div>
        <div><dt>Начато</dt><dd><time dateTime={data.started_at}>{dateFormatter.format(new Date(data.started_at))}</time></dd></div>
        <div><dt>Завершено</dt><dd><time dateTime={data.completed_at}>{dateFormatter.format(new Date(data.completed_at))}</time></dd></div>
      </dl>
    </header>

    <ReportNavigation sections={[
      { id: 'report-summary', label: 'Краткий вывод' },
      ...(result?.criterion_scores.length ? [{ id: 'report-criteria', label: 'Критерии', count: result.criterion_scores.length }] : []),
      ...(result ? [{ id: 'report-insights', label: 'Сильные стороны и риски' }] : []),
      ...(result?.follow_up_questions.length ? [{ id: 'report-follow-up', label: 'Следующий этап' }] : []),
      { id: 'interview-answers', label: 'Ответы', count: data.answers.length },
    ]} />

    {interview.isError ? <div className={styles.refreshNotice} role="status">
      <p>Не удалось обновить данные. Показан последний загруженный отчёт.</p>
      <Button variant="ghost" size="sm" loading={interview.isFetching} onClick={() => void interview.refetch()}>Обновить данные</Button>
    </div> : null}
    <CandidateReport report={data.report}
      onRetry={() => rebuild.mutate({ id: sessionId, version: data.report.version })}
      isRetrying={rebuild.isPending && rebuild.variables?.id === sessionId}
      retryError={rebuild.isError && rebuild.variables?.id === sessionId && rebuild.variables.version === data.report.version}
    />
    <section id="interview-answers" className={styles.answers} aria-labelledby="answers-title">
      <div className={styles.sectionHeading}>
        <div><h2 id="answers-title">Ответы кандидата <span>{data.answers.length}</span></h2><p>Исходные ответы и их подробный разбор. Выберите вопрос, чтобы прочитать.</p></div>
        <a className={styles.toSummary} href="#report-summary">К выводу ↑</a>
      </div>
      {data.answers.length === 0 ? <p className={styles.empty}>Ответов пока нет.</p> : <ol className={styles.answerList}>
        {data.answers.map((answer) => {
          const quoteHash = quoteReturn?.sessionId === sessionId && quoteReturn.answerHash === `#answer-${answer.position + 1}` ? quoteReturn.quoteHash : null
          return <li key={answer.id}>
          <details id={`answer-${answer.position + 1}`} className={styles.answerCard}>
            <summary className={styles.questionHeading}>
              <span className={styles.questionNumber} aria-hidden="true">{String(answer.position + 1).padStart(2, '0')}</span>
              <div className={styles.questionText}>
                <h3><span className="sr-only">Вопрос {answer.position + 1}. </span>{answer.question}</h3>
                <span className={styles.answerMeta}>
                  <span>{answer.answer_type === 'voice' ? <Mic size={13} aria-hidden="true" /> : <FileText size={13} aria-hidden="true" />}{answer.answer_type === 'voice' ? 'Голосовой ответ' : 'Текстовый ответ'}</span>
                  {answer.transcription?.status === 'permanent_failed' ? <span className={styles.failed}>Ошибка распознавания</span> : answer.transcription?.status === 'pending' ? <span className={styles.pending}>Распознаём…</span> : answer.evaluation.status === 'permanent_failed' ? <span className={styles.failed}>Ошибка проверки</span> : answer.evaluation.status === 'pending' ? <span className={styles.pending}>Проверяем…</span> : null}
                </span>
              </div>
              <ChevronDown className={styles.answerChevron} size={18} aria-hidden="true" />
            </summary>
            <div className={styles.answerContent}>
              {quoteHash ? <a className={styles.sourceReturn} href={quoteHash}><ArrowLeft size={14} aria-hidden="true" />Вернуться к цитате</a> : null}
              <div className={styles.answerBody}>
                <p className={styles.answerLabel}>{answer.answer_type === 'voice' ? 'Расшифровка ответа' : 'Ответ кандидата'}</p>
                {answer.answer_type === 'text' ? <p>{answer.text || 'Ответ отсутствует'}</p>
                  : answer.transcription?.status === 'succeeded' ? <p>{answer.transcription.text}</p>
                  : answer.transcription?.status === 'permanent_failed' ? <p className={styles.failed}>Не удалось распознать голосовой ответ.</p>
                  : <p className={styles.pending}>Распознаём голосовой ответ…</p>}
              </div>
              <AnswerEvaluation sessionId={sessionId} answerId={answer.id} evaluation={answer.evaluation} />
              <div className={styles.answerFooter}><time dateTime={answer.answered_at}>{dateFormatter.format(new Date(answer.answered_at))}</time><a href={quoteHash ?? '#report-summary'}>{quoteHash ? 'К цитате в отчёте ↑' : 'К краткому выводу ↑'}</a></div>
            </div>
          </details>
        </li>})}
      </ol>}
    </section>

    <details className={styles.profile}>
      <summary><UserRound size={18} aria-hidden="true" /><span>Данные кандидата<small>Telegram-профиль и история контактов</small></span><ChevronDown size={18} aria-hidden="true" /></summary>
      <dl>
        <div><dt>Telegram ID</dt><dd>{data.candidate.telegram_user_id}</dd></div>
        <div><dt>Язык</dt><dd>{data.candidate.language_code?.toUpperCase() || 'Не указан'}</dd></div>
        <div><dt>Premium</dt><dd>{data.candidate.is_premium ? 'Да' : 'Нет'}</dd></div>
        <div><dt>Первый контакт</dt><dd><time dateTime={data.candidate.first_seen_at}>{dateFormatter.format(new Date(data.candidate.first_seen_at))}</time></dd></div>
        <div><dt>Последний контакт</dt><dd><time dateTime={data.candidate.last_seen_at}>{dateFormatter.format(new Date(data.candidate.last_seen_at))}</time></dd></div>
      </dl>
    </details>
  </main>
}
