import { CalendarDays, LogOut, Mail } from 'lucide-react'

import { useSession } from '@/features/session'
import { Button } from '@/shared/ui'
import styles from './SettingsPage.module.css'

export function SettingsPage() {
  const session = useSession()
  const createdAt = session.user?.created_at
    ? new Intl.DateTimeFormat('ru', { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date(session.user.created_at))
    : '—'

  return (
    <main className={styles.page}>
      <header>
        <p>Аккаунт</p>
        <h1>Настройки</h1>
      </header>

      <section className={styles.card}>
        <div className={styles.title}>
          <h2>Профиль</h2>
          <p>Основная информация вашего аккаунта.</p>
        </div>
        <dl>
          <div><dt><Mail /> Email</dt><dd>{session.user?.email}</dd></div>
          <div><dt><CalendarDays /> Дата регистрации</dt><dd>{createdAt}</dd></div>
        </dl>
      </section>

      <section className={styles.logoutCard}>
        <div><h2>Завершить сессию</h2><p>Для продолжения работы потребуется снова войти.</p></div>
        <Button variant="danger" onClick={() => void session.logout()}><LogOut size={18} /> Выйти</Button>
      </section>
    </main>
  )
}
