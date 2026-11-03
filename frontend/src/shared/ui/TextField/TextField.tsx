import { forwardRef, type InputHTMLAttributes } from 'react'

import { cn } from '@/shared/lib/cn'
import styles from './TextField.module.css'

interface TextFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
  error?: string
  hint?: string
}

export const TextField = forwardRef<HTMLInputElement, TextFieldProps>(function TextField(
  { className, error, hint, id, label, ...props },
  ref,
) {
  const fieldId = id ?? props.name
  const describedBy = error ? `${fieldId}-error` : hint ? `${fieldId}-hint` : undefined

  return (
    <label className={cn(styles.field, className)} htmlFor={fieldId}>
      <span className={styles.label}>{label}</span>
      <input
        ref={ref}
        id={fieldId}
        className={cn(styles.input, error && styles.invalid)}
        aria-invalid={Boolean(error)}
        aria-describedby={describedBy}
        {...props}
      />
      {error ? (
        <span className={styles.error} id={`${fieldId}-error`} role="alert">
          {error}
        </span>
      ) : hint ? (
        <span className={styles.hint} id={`${fieldId}-hint`}>
          {hint}
        </span>
      ) : null}
    </label>
  )
})
