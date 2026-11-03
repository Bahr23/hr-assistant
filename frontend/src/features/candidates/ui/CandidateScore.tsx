import type { components } from '@/shared/api'
import styles from './CandidateScore.module.css'

type Report = components['schemas']['CandidateReport']
const number = new Intl.NumberFormat('ru-RU', { minimumFractionDigits: 1, maximumFractionDigits: 1 })

export function CandidateScore({ report }: { report?: Report }) {
  if (!report?.result || report.total_criteria === 0) return null

  return <div className={styles.score} aria-label="Общий балл кандидата">
    <div className={styles.value}>
      <span>Общий балл</span>
      <strong className={report.overall_score === null ? styles.missing : undefined}>
        {report.overall_score === null ? 'Недостаточно данных' : <>{number.format(report.overall_score)} <small>из 10</small></>}
      </strong>
    </div>
    <div className={styles.coverage}>
      <p>Оценено {report.scored_criteria} из {report.total_criteria} критериев</p>
      <div className={styles.coverageMeter} role="meter" aria-label="Покрытие критериев" aria-valuemin={0} aria-valuemax={report.total_criteria} aria-valuenow={report.scored_criteria} aria-valuetext={`Оценено ${report.scored_criteria} из ${report.total_criteria} критериев`}>
        <span style={{ width: `${report.scored_criteria / report.total_criteria * 100}%` }} />
      </div>
      <p className={styles.explanation}>{report.overall_score === null ? 'Для общего балла недостаточно данных.' : 'Среднее по оценённым критериям'}</p>
    </div>
    {report.result_version !== report.version ? <p className={styles.previous}>Предыдущий результат</p> : null}
  </div>
}
