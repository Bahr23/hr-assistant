import { useId } from 'react'

import styles from './EvaluationGuideField.module.css'

type Props = { value: string; onChange: (value: string) => void; disabled?: boolean }

export function EvaluationGuideField({ value, onChange, disabled }: Props) {
  const id = useId()
  return (
    <div className={styles.field}>
      <label htmlFor={id}>Что искать в ответе</label>
      <p id={`${id}-hint`}>
        Опишите, что должен содержать хороший ответ и какие признаки нежелательны. AI сопоставит ответ кандидата с этими ориентирами и объяснит результат. Поле необязательное.
      </p>
      <textarea
        id={id}
        aria-describedby={`${id}-hint`}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        rows={4}
        maxLength={4000}
        placeholder="Хороший ответ: конкретный пример, действия кандидата и полученный результат. Нежелательные признаки: давление на клиента, обещания, которые невозможно выполнить."
      />
      <small>{Array.from(value).length.toLocaleString('ru-RU')} / 4 000</small>
    </div>
  )
}
