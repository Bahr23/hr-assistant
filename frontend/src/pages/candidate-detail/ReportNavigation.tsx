import { useEffect, useRef, useState } from 'react'
import styles from './CandidateDetailPage.module.css'

type Section = { id: string; label: string; count?: number }

export function ReportNavigation({ sections }: { sections: Section[] }) {
  const [active, setActive] = useState('report-summary')
  const navigation = useRef<HTMLElement>(null)
  const sectionIds = sections.map((section) => section.id).join(',')

  useEffect(() => {
    let frame = 0
    const update = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const ids = sectionIds.split(',')
        const boundary = (navigation.current?.offsetHeight ?? 60) + 32
        const current = ids.slice().reverse().find((id) => {
          const section = document.getElementById(id)
          return section && section.getBoundingClientRect().top <= boundary
        })
        setActive(current ?? ids[0] ?? 'report-summary')
      })
    }
    update()
    window.addEventListener('scroll', update, { passive: true })
    window.addEventListener('resize', update)
    document.addEventListener('toggle', update, true)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('scroll', update)
      window.removeEventListener('resize', update)
      document.removeEventListener('toggle', update, true)
    }
  }, [sectionIds])

  function selectSection(id: string) {
    const section = document.getElementById(id)
    if (!section) return
    setActive(id)
    window.location.hash = id
    section.tabIndex = -1
    section.focus({ preventScroll: true })
    section.scrollIntoView({ block: 'start' })
  }

  return <nav ref={navigation} className={styles.sectionNav} aria-label="Разделы отчёта">
    <div className={styles.sectionLinks}>
      {sections.map((section) => <a key={section.id} href={`#${section.id}`} aria-current={active === section.id ? 'location' : undefined}>
        {section.label}{section.count !== undefined ? <span>{section.count}</span> : null}
      </a>)}
    </div>
    <label className={styles.mobileSectionNav}>
      <span>Раздел отчёта</span>
      <select value={active} onChange={(event) => selectSection(event.target.value)}>
        {sections.map((section) => <option key={section.id} value={section.id}>{section.label}{section.count !== undefined ? ` · ${section.count}` : ''}</option>)}
      </select>
    </label>
  </nav>
}
