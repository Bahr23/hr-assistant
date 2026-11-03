import { LoaderCircle } from 'lucide-react'

import styles from './Spinner.module.css'

export function Spinner({ label = 'Загрузка' }: { label?: string }) {
  return (
    <div className={styles.root} role="status">
      <LoaderCircle aria-hidden="true" />
      <span>{label}</span>
    </div>
  )
}
