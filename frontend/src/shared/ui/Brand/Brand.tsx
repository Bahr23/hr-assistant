import { Sparkles } from 'lucide-react'
import { Link } from 'react-router-dom'

import { cn } from '@/shared/lib/cn'
import styles from './Brand.module.css'

export function Brand({ compact = false, className }: { compact?: boolean; className?: string }) {
  return (
    <Link className={cn(styles.brand, className)} to="/" aria-label="HR Assistant — главная">
      <span className={styles.mark} aria-hidden="true">
        <Sparkles size={20} strokeWidth={2.5} />
      </span>
      {!compact ? <span>HR Assistant</span> : null}
    </Link>
  )
}
