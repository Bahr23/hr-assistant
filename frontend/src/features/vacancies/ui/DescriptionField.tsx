import { forwardRef, useId, type TextareaHTMLAttributes } from 'react'
import styles from './EvaluationGuideField.module.css'

export const DescriptionField = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(function DescriptionField(props, ref) {
  const id = useId()
  return <div className={styles.field}>
    <label htmlFor={id}>Описание вакансии</label>
    <p id={`${id}-hint`}>Расскажите об обязанностях, требованиях и условиях работы. Описание увидит кандидат, а AI будет учитывать его при анализе интервью.</p>
    <textarea {...props} ref={ref} id={id} aria-describedby={`${id}-hint`} rows={6} maxLength={10000} placeholder="Обязанности, требования и условия работы" />
    <small>Необязательно · до 10 000 символов</small>
  </div>
})
