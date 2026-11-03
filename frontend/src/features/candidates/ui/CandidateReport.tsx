import { ArrowUpRight, Check, ChevronDown, CircleAlert, ListChecks, LoaderCircle, RotateCcw, Sparkles } from 'lucide-react'
import { useId, type ReactNode } from 'react'
import { FollowUpQuestions } from './FollowUpQuestions'
import type { components } from '@/shared/api'
import { Button } from '@/shared/ui'
import { CandidateScore } from './CandidateScore'
import styles from './CandidateReport.module.css'

type Report = components['schemas']['CandidateReport']
type Result = components['schemas']['CandidateReportResult']
type Evidence = Result['criterion_scores'][number]['evidence']
const number = new Intl.NumberFormat('ru-RU', { minimumFractionDigits: 1, maximumFractionDigits: 1 })
const severity = { low: 'Низкая значимость', medium: 'Средняя значимость', high: 'Высокая значимость' }

function Quotes({ evidence }: { evidence: Evidence }) {
  const id = useId()
  return evidence.map((item, index) => <blockquote key={index}>
    <p>{item.quote}</p>
    <a id={`report-quote-${id}-${index}`} href={`#answer-${item.question_number}`}>Ответ на вопрос {item.question_number}<ArrowUpRight size={14} aria-hidden="true" /></a>
  </blockquote>)
}

function ReportDetails({ title, label = 'Цитаты из ответов', children }: { title: string; label?: string; children: ReactNode }) {
  return <details className={styles.cardDetails}>
    <summary aria-label={`${label}: ${title}`}>{label}<ChevronDown size={15} aria-hidden="true" /></summary>
    <div className={styles.cardDetailsBody}>{children}</div>
  </details>
}

function CriterionRow({ criterion }: { criterion: Result['criterion_scores'][number] }) {
  const id = useId()
  const score = criterion.status === 'insufficient_data' ? null : criterion.score
  return <details className={styles.criterion}>
    <summary className={styles.criterionHeading} aria-label={`Обоснование оценки: ${criterion.name}`} aria-describedby={`${id}-score ${id}-preview`}>
      <h3>{criterion.name}</h3>
      <p id={`${id}-preview`} className={styles.criterionPreview}>{criterion.summary}</p>
      <div className={styles.criterionRating}>
        <span id={`${id}-score`} className={score === null ? styles.noScore : styles.criterionScore}>{score === null ? 'Недостаточно данных' : <>{number.format(score)} <small>/ 10</small></>}</span>
        <div className={`${styles.scoreTrack} ${score === null ? styles.unscoredTrack : ''}`} aria-hidden="true"><span style={{ width: `${score === null ? 0 : score * 10}%` }} /></div>
      </div>
      <ChevronDown className={styles.criterionChevron} size={16} aria-hidden="true" />
    </summary>
    {criterion.evidence.length > 0 || criterion.missing_information.length > 0 ? <div className={styles.criterionBody}>
      <Quotes evidence={criterion.evidence} />
      {criterion.missing_information.length > 0 ? <div className={styles.missing}><strong>Не хватает информации</strong><ul>{criterion.missing_information.map((item, index) => <li key={index}>{item}</li>)}</ul></div> : null}
    </div> : null}
  </details>
}

type ReportProps = { report?: Report; onRetry?: () => void; isRetrying?: boolean; retryError?: boolean }

export function CandidateReport({ report, onRetry, isRetrying = false, retryError = false }: ReportProps) {
  const id = useId()
  const result = report?.result
  const pending = report?.status === 'pending' || isRetrying
  return <div className={styles.report}>
    <section id="report-summary" className={`${styles.panel} ${styles.overview}`} aria-labelledby={`${id}-summary`}>
      <div className={styles.sectionHeading}><span className={styles.sectionIcon}><Sparkles size={18} aria-hidden="true" /></span><h2 id={`${id}-summary`}>Краткий вывод</h2><span className={styles.label}>AI-отчёт</span></div>
      {pending ? <div className={styles.notice} role="status">
        <LoaderCircle className={styles.processingIcon} size={18} aria-hidden="true" />
        <div><strong>Готовим отчёт.</strong><p>{result ? 'Обновляем анализ. Предыдущий результат остаётся доступен.' : 'Ожидаем обработки всех ответов и общего анализа. Страница обновится автоматически.'}</p></div>
      </div> : null}
      {report?.status === 'permanent_failed' && !pending ? <div className={styles.failure} role="alert">
        <CircleAlert size={18} aria-hidden="true" />
        <div><strong>Не удалось собрать новую версию отчёта.</strong>
          <p>{result ? 'Предыдущий результат сохранён. Можно повторить сборку.' : 'Ответы кандидата сохранены. Можно повторить сборку или прочитать интервью ниже.'}</p>
          {onRetry ? <Button size="sm" variant="ghost" onClick={onRetry}><RotateCcw size={14} aria-hidden="true" />Повторить сборку</Button> : null}
          {retryError ? <p className={styles.retryError}>Не удалось запустить сборку. Попробуйте ещё раз.</p> : null}
        </div>
      </div> : null}
      <div className={styles.summaryLayout}>
        <CandidateScore report={report} />
        <div className={styles.summaryText}>
          {result && report ? <>
            {report.result_version !== report.version ? <p className={styles.previousVersion}>Показан предыдущий успешный отчёт — версия {report.result_version}.</p> : null}
            <p className={styles.summary}>{result.summary}</p>
          </> : !pending && report?.status !== 'permanent_failed' ? <p className={styles.caption}>Отчёт ещё не сформирован. Ответы кандидата доступны ниже.</p> : null}
        </div>
      </div>
      {result?.analysis_completeness.status === 'partial' ? <div className={styles.partial}>
        <CircleAlert size={18} aria-hidden="true" />
        <div><strong>Анализ выполнен по неполным данным</strong>
          <p>Полностью обработано ответов: {result.analysis_completeness.processed_answers} из {result.analysis_completeness.total_answers}. Ошибки обработки не означают низкую оценку.</p>
          {result.analysis_completeness.issues.length > 0 ? <ul>{result.analysis_completeness.issues.map((issue, index) => <li key={index}>{issue}</li>)}</ul> : null}
        </div>
      </div> : null}
    </section>
    {result ? <>
      {result.criterion_scores.length > 0 ? <section id="report-criteria" className={styles.panel} aria-labelledby={`${id}-criteria`}>
        <div className={styles.sectionHeading}><span className={styles.sectionIcon}><ListChecks size={18} aria-hidden="true" /></span><h2 id={`${id}-criteria`}>Оценки по критериям</h2><span className={styles.count}>{result.criterion_scores.length}</span></div>
        <p className={styles.sectionDescription}>Выберите критерий, чтобы прочитать полный разбор и цитаты из ответов.</p>
        <div className={styles.criteria}>{result.criterion_scores.map((criterion) => <CriterionRow key={criterion.criterion_id} criterion={criterion} />)}</div>
      </section> : null}
      <div id="report-insights" className={styles.columns}>
        <section className={`${styles.panel} ${styles.strengths}`} aria-labelledby={`${id}-strengths`}>
          <div className={styles.sectionHeading}><span className={styles.sectionIcon}><Check size={18} aria-hidden="true" /></span><h2 id={`${id}-strengths`}>Сильные стороны</h2><span className={styles.count}>{result.strengths.length}</span></div>
          {result.strengths.length === 0 ? <p className={styles.caption}>Подтверждённые сильные стороны не выделены.</p> : <div className={styles.insights}>{result.strengths.map((strength, index) => <article key={index} className={styles.insight}>
            <h3>{strength.title}</h3>
            <p className={styles.cardDescription}>{strength.description}</p>
            {strength.evidence.length > 0 ? <ReportDetails title={strength.title}><Quotes evidence={strength.evidence} /></ReportDetails> : null}
          </article>)}</div>}
        </section>
        <section className={`${styles.panel} ${styles.concerns}`} aria-labelledby={`${id}-concerns`}>
          <div className={styles.sectionHeading}><span className={styles.sectionIcon}><CircleAlert size={18} aria-hidden="true" /></span><h2 id={`${id}-concerns`}>Сомнения и риски</h2><span className={styles.count}>{result.concerns.length}</span></div>
          {result.concerns.length === 0 ? <p className={styles.caption}>Отдельные сомнения не выделены.</p> : <div className={styles.insights}>{result.concerns.map((concern, index) => <article key={index} className={styles.insight}>
            <span className={`${styles.severity} ${styles[concern.severity]}`}>{severity[concern.severity]}</span>
            <h3>{concern.title}</h3>
            <p className={styles.cardDescription}>{concern.description}</p>
            {concern.evidence.length > 0 ? <ReportDetails title={concern.title}><Quotes evidence={concern.evidence} /></ReportDetails> : null}
          </article>)}</div>}
        </section>
      </div>
      {result.follow_up_questions.length > 0 ? <FollowUpQuestions key={report?.result_version} questions={result.follow_up_questions} /> : null}
    </> : null}
  </div>
}
