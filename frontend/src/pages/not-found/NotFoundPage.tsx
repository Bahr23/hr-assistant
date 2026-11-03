import { Link } from 'react-router-dom'

import { Brand } from '@/shared/ui'
import styles from './NotFoundPage.module.css'

export function NotFoundPage() {
  return (
    <main className={styles.page}>
      <Brand />
      <p>404</p>
      <h1>ЭТА СТРАНИЦА<br />УЖЕ НА ИНТЕРВЬЮ</h1>
      <Link to="/">Вернуться в кабинет</Link>
    </main>
  )
}
