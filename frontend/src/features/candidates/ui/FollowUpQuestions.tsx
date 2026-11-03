import { Check, Copy, MessageCircle } from 'lucide-react'
import { useId, useState } from 'react'
import type { components } from '@/shared/api'
import { Button } from '@/shared/ui'
import styles from './CandidateReport.module.css'

type Questions = components['schemas']['CandidateReportResult']['follow_up_questions']

export function FollowUpQuestions({ questions }: { questions: Questions }) {
  const id = useId()
  const [copyState, setCopyState] = useState<'idle' | 'copying' | 'copied' | 'failed'>('idle')

  async function copyQuestions() {
    setCopyState('copying')
    try {
      const text = questions.map((item, index) => `${index + 1}. ${item.question}\nЗачем уточнить: ${item.reason}`).join('\n\n')
      await navigator.clipboard.writeText(text)
      setCopyState('copied')
    } catch {
      setCopyState('failed')
    }
  }

  return <section id="report-follow-up" className={styles.panel} aria-labelledby={id}>
    <div className={styles.followUpHeader}>
      <div className={styles.sectionHeading}><span className={styles.sectionIcon}><MessageCircle size={18} aria-hidden="true" /></span><h2 id={id}>Вопросы для следующего этапа</h2><span className={styles.count}>{questions.length}</span></div>
      <Button variant="ghost" size="sm" className={styles.copyQuestions} disabled={copyState === 'copying'} onClick={() => void copyQuestions()}>
        {copyState === 'copied' ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}Скопировать вопросы
      </Button>
    </div>
    <p className={styles.sectionDescription}>Помогут уточнить то, что осталось за рамками интервью.</p>
    {copyState === 'copied' ? <p className={styles.copySuccess} role="status">Вопросы скопированы вместе с пояснениями.</p> : null}
    {copyState === 'failed' ? <p className={styles.copyError} role="alert">Не удалось скопировать. Выделите вопросы ниже и скопируйте вручную.</p> : null}
    <ol className={styles.followUp}>{questions.map((question, index) => <li key={index}>
      <span className={styles.questionNumber} aria-hidden="true">{String(index + 1).padStart(2, '0')}</span>
      <div><p className={styles.question}>{question.question}</p><p className={styles.caption}>{question.reason}</p></div>
    </li>)}</ol>
  </section>
}
