import { LoaderCircle } from 'lucide-react'
import { forwardRef, type ButtonHTMLAttributes } from 'react'

import { cn } from '@/shared/lib/cn'
import styles from './Button.module.css'

type ButtonVariant = 'primary' | 'brand' | 'highIntent' | 'ghost' | 'white' | 'danger'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: 'sm' | 'md'
  loading?: boolean
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { children, className, disabled, loading = false, size = 'md', variant = 'primary', ...props },
  ref,
) {
  return (
    <button
      ref={ref}
      className={cn(styles.button, styles[variant], styles[size], className)}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? <LoaderCircle className={styles.spinner} aria-hidden="true" /> : null}
      {children}
    </button>
  )
})
