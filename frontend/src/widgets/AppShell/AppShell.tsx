import { BriefcaseBusiness, LayoutDashboard, Users } from 'lucide-react'
import { Link, NavLink, Outlet } from 'react-router-dom'

import { useSession } from '@/features/session'
import { cn } from '@/shared/lib/cn'
import { Brand } from '@/shared/ui'
import styles from './AppShell.module.css'

export function AppShell() {
  const session = useSession()
  const initials = session.user?.email.slice(0, 1).toUpperCase()

  return (
    <div className={styles.shell}>
      <aside className={styles.sidebar}>
        <div className={styles.logoSlot} data-tooltip="HR Assistant">
          <Brand compact className={styles.brand} />
        </div>
        <nav className={styles.nav} aria-label="Основная навигация">
          <NavLink to="/vacancies" aria-label="Вакансии" data-tooltip="Вакансии" className={({ isActive }) => cn(styles.navItem, isActive && styles.active)}>
            <BriefcaseBusiness aria-hidden="true" />
          </NavLink>
          <NavLink to="/candidates" aria-label="Кандидаты" data-tooltip="Кандидаты" className={({ isActive }) => cn(styles.navItem, isActive && styles.active)}>
            <Users aria-hidden="true" />
          </NavLink>
          <span className={cn(styles.navItem, styles.disabled)} data-tooltip="Обзор — скоро" aria-label="Обзор — скоро"><LayoutDashboard /></span>
        </nav>
        <div className={styles.profile}>
          <Link to="/settings" className={styles.avatar} data-tooltip="Настройки профиля" aria-label="Открыть настройки профиля">{initials || 'HR'}</Link>
        </div>
      </aside>
      <div className={styles.main}>
        <Outlet />
      </div>
    </div>
  )
}
