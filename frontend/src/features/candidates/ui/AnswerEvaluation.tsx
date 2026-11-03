import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, ChevronDown, CircleAlert, CircleHelp, MessageCircle, Minus } from 'lucide-react'
import { useId, type ReactNode } from 'react'

import type { components } from '@/shared/api'
import { Button } from '@/shared/ui'
import { candidatesApi } from '../api/candidatesApi'
import { candidateKeys } from '../model/queries'
import styles from './AnswerEvaluation.module.css'

type Evaluation = components['schemas']['AnswerEvaluation']
type Check = components['schemas']['AnswerEvaluationCheck']
type Props = { sessionId: string; answerId: string; evaluation: Evaluation }

const failureMessages: Record<string, string> = {
  invalid_result: 'Результат AI не прошёл проверку формата или дословности цитат.',
  openrouter_unavailable: 'Сервис AI временно недоступен.',
  openrouter_auth_failed: 'Сервис AI недоступен из-за настроек доступа или оплаты.',
  openrouter_rejected: 'Сервис AI отклонил запрос.',
  model_refusal: 'Модель не выполнила проверку этого ответа.',
  invalid_input: 'Не удалось подготовить данные для проверки.',
  unsupported_version: 'Эта версия проверки не поддерживается сервисом AI.',
}

type GroupTone = 'attention' | 'risk' | 'positive' | 'neutral' | 'questions'
const groupIcons = { attention: CircleHelp, risk: CircleAlert, positive: Check, neutral: Minus, questions: MessageCircle }

function EvaluationGroup({ title, count, tone, children }: { title: string; count: number; tone: GroupTone; children: ReactNode }) {
  const id = useId()
  const Icon = groupIcons[tone]
  return <details className={`${styles.group} ${styles[tone]}`}>
    <summary aria-label={title} aria-describedby={id}>
      <span className={styles.groupIcon}><Icon size={16} aria-hidden="true" /></span>
      <h5>{title}</h5><span className={styles.groupCount} aria-hidden="true">{count}</span><span id={id} className="sr-only">{`Пунктов: ${count}`}</span><ChevronDown size={16} className={styles.chevron} aria-hidden="true" />
    </summary>
    <div className={styles.groupBody}>{children}</div>
  </details>
}

function CheckGroup({ title, checks, tone }: { title: string; checks: Check[]; tone: GroupTone }) {
  if (!checks.length) return null
  return (
    <EvaluationGroup title={title} count={checks.length} tone={tone}>
      <ul>
        {checks.map((check, index) => (
          <li key={index}>
            <div className={styles.checkHeading}><strong>{check.criterion}</strong>
              <span className={`${styles.status} ${check.status === 'found' ? (check.type === 'expected' ? styles.found : styles.unwanted) : check.status === 'unclear' ? styles.unclear : ''}`}>{check.status === 'found' ? 'Найдено' : check.status === 'unclear' ? 'Неясно' : 'Не найдено'}</span>
            </div>
            <p>{check.explanation}</p>
            {check.evidence.map((quote, quoteIndex) => <blockquote key={quoteIndex}>{quote}</blockquote>)}
          </li>
        ))}
      </ul>
    </EvaluationGroup>
  )
}

export function AnswerEvaluation({ sessionId, answerId, evaluation }: Props) {
  const queryClient = useQueryClient()
  const retry = useMutation({
    mutationFn: (version: number) => candidatesApi.retryEvaluation(sessionId, answerId, version),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: candidateKeys.detail(sessionId) })
    },
  })
  const result = evaluation.result
  if (evaluation.status === 'skipped') {
    return <p className={styles.skipped}>{evaluation.reason === 'transcription_failed'
      ? 'Проверка недоступна: не удалось получить текст голосового ответа.'
      : 'Проверка не задана для этого вопроса.'}</p>
  }
  return (
    <section className={styles.evaluation} aria-label="Проверка ответа">
      <h4>Проверка ответа</h4>
      {evaluation.status === 'pending' ? <p role="status" className={styles.pending}>
        {evaluation.reason === 'waiting_transcription' ? 'Проверим ответ после распознавания голосового сообщения.' : 'Проверяем ответ'}
      </p> : null}
      {evaluation.status === 'permanent_failed' ? <div className={styles.failure}>
        <p role="status">Не удалось выполнить проверку</p>
        {evaluation.errorCode && failureMessages[evaluation.errorCode] ? <p>{failureMessages[evaluation.errorCode]}</p> : null}
        {(evaluation.attempts ?? 0) > 0 ? <p>Выполнено попыток: {evaluation.attempts}. Ошибка обработки не снижает оценку кандидата.</p> : null}
        {evaluation.version !== null ? <Button variant="ghost" loading={retry.isPending} onClick={() => retry.mutate(evaluation.version!)}>Повторить проверку</Button> : null}
        {retry.isError ? <p role="alert">Не удалось запустить проверку. Попробуйте ещё раз.</p> : null}
      </div> : null}
      {result ? <>
        {evaluation.resultVersion !== evaluation.version ? <p className={styles.previous}>Показан предыдущий успешный результат.</p> : null}
        <p className={styles.summary}>{result.summary}</p>
        <div className={styles.groups}>
          <CheckGroup title="Не подтверждено или требует уточнения" tone="attention" checks={result.checks.filter((check) => (check.type === 'expected' && check.status !== 'found') || (check.type === 'unwanted' && check.status === 'unclear'))} />
          <CheckGroup title="Найденные нежелательные признаки" tone="risk" checks={result.checks.filter((check) => check.type === 'unwanted' && check.status === 'found')} />
          <CheckGroup title="Найденные ожидаемые элементы" tone="positive" checks={result.checks.filter((check) => check.type === 'expected' && check.status === 'found')} />
          <CheckGroup title="Нежелательные признаки не найдены в ответе" tone="neutral" checks={result.checks.filter((check) => check.type === 'unwanted' && check.status === 'not_found')} />
          {result.follow_up_questions.length ? <EvaluationGroup title="Уточняющие вопросы" count={result.follow_up_questions.length} tone="questions">
            <ol>{result.follow_up_questions.map((question, index) => <li key={index}>{question}</li>)}</ol>
          </EvaluationGroup> : null}
        </div>
      </> : null}
    </section>
  )
}
