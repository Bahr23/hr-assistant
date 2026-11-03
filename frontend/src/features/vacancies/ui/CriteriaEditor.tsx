import { useQuery } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useId, useState } from 'react'
import { moveItem } from '@/shared/lib/moveItem'
import type { Criterion } from '../model/criteria'
import { Button, Modal, TextField } from '@/shared/ui'
import { vacanciesApi } from '../api/vacanciesApi'
import styles from './CriteriaEditor.module.css'

type Props = { value: Criterion[]; onChange: (values: Criterion[]) => void; disabled?: boolean }
export function CriteriaEditor({ value, onChange, disabled = false }: Props) {
  const id = useId()
  const [catalogOpen, setCatalogOpen] = useState(false)
  const [selected, setSelected] = useState<string[]>([])
  const catalog = useQuery({ queryKey: ['evaluation-criteria', 'templates'], queryFn: vacanciesApi.templates, staleTime: Infinity })
  const update = (values: Criterion[]) => onChange(values.map((c, position) => ({ ...c, position })))
  const patch = (criterionId: string, fields: Partial<Criterion>) => update(value.map((c) => c.id === criterionId ? { ...c, ...fields } : c))
  const move = (index: number, offset: number) => {
    update(moveItem(value, index, index + offset))
  }
  const addTemplates = () => {
    const additions = (catalog.data?.items ?? []).filter((t) => selected.includes(t.id)).map((t) => ({ ...t, id: crypto.randomUUID(), sourceTemplateId: t.id, position: 0 }))
    update([...value, ...additions].slice(0, 20))
    setSelected([])
    setCatalogOpen(false)
  }
  return <section className={styles.section} aria-labelledby={`${id}-heading`}>
    <div className={styles.heading}><h2 id={`${id}-heading`}>Критерии оценки кандидата</h2><span>{value.length} / 20</span></div>
    <p className={styles.hint}>Выберите, по каким критериям оценивать всех кандидатов этой вакансии. По каждому критерию AI поставит балл от 0 до 10 и объяснит оценку.</p>
    {value.length === 0 ? <p className={styles.empty}>Без критериев будет доступен текстовый отчёт без баллов.</p> : null}
    <ol className={styles.list}>
      {value.map((c, index) => {
        const duplicate = value.some((other) => other.id !== c.id && other.title.trim().toLocaleLowerCase('ru') === c.title.trim().toLocaleLowerCase('ru'))
        const guideId = `${id}-${c.id}-guide`
        return <li key={c.id} className={styles.criterion}>
          <div className={styles.toolbar}><span>Критерий {index + 1}</span><div>
            <button type="button" disabled={disabled || index === 0} onClick={() => move(index, -1)} aria-label={`Поднять критерий ${index + 1}`}><ArrowUp size={17} /></button>
            <button type="button" disabled={disabled || index === value.length - 1} onClick={() => move(index, 1)} aria-label={`Опустить критерий ${index + 1}`}><ArrowDown size={17} /></button>
            <button type="button" disabled={disabled} onClick={() => update(value.filter((other) => other.id !== c.id))} aria-label={`Удалить критерий ${index + 1}`}><Trash2 size={17} /></button>
          </div></div>
          <TextField id={`${id}-${c.id}-title`} label="Название" value={c.title} disabled={disabled} maxLength={120} required onChange={(e) => patch(c.id, { title: e.target.value })} />
          {duplicate && c.title.trim() ? <p className={styles.warning} role="status">Критерий с таким названием уже добавлен. Можно уточнить название.</p> : null}
          <div className={styles.guide}><label htmlFor={guideId}>Как оценивать</label>
            <p id={`${guideId}-hint`}>Опишите, какие знания, действия и примеры подтверждают высокую оценку, а какие ответы должны её снижать.</p>
            <textarea id={guideId} aria-describedby={`${guideId}-hint`} rows={4} value={c.evaluationGuide} disabled={disabled} required maxLength={2000} onChange={(e) => patch(c.id, { evaluationGuide: e.target.value })} />
          </div>
          {!c.title.trim() || !c.evaluationGuide.trim() ? <p className={styles.hint}>Заполните название и инструкцию для этого критерия.</p> : null}
        </li>
      })}
    </ol>
    <div className={styles.actions}>
      <Button type="button" disabled={disabled || value.length >= 20} onClick={() => { setSelected([]); setCatalogOpen(true) }}>Выбрать из каталога</Button>
      <Button type="button" variant="ghost" disabled={disabled || value.length >= 20} onClick={() => update([...value, { id: crypto.randomUUID(), title: '', evaluationGuide: '', sourceTemplateId: null, position: value.length }])}><Plus size={17} /> Свой критерий</Button>
    </div>
    {catalogOpen ? <Modal open title="Выберите критерии" description="Шаблоны можно изменить после добавления в вакансию." onClose={() => setCatalogOpen(false)}>
      <div className={styles.catalogBody}>
        {catalog.isPending ? <p role="status">Загружаем каталог…</p> : null}
        {catalog.isError ? <div role="alert"><p>Не удалось загрузить каталог.</p><Button type="button" onClick={() => void catalog.refetch()}>Повторить</Button></div> : null}
        <div className={styles.catalog}>
          {catalog.data?.items.map((t) => <label key={t.id} className={styles.template}>
            <input type="checkbox" checked={selected.includes(t.id)} disabled={!selected.includes(t.id) && value.length + selected.length >= 20} onChange={(e) => setSelected((ids) => e.target.checked ? [...ids, t.id] : ids.filter((item) => item !== t.id))} />
            <span><strong>{t.title}</strong><small>{t.evaluationGuide}</small></span>
          </label>)}
        </div>
        <div className={styles.actions}><Button type="button" onClick={addTemplates} disabled={selected.length === 0}>Добавить выбранные ({selected.length})</Button></div>
      </div>
    </Modal> : null}
  </section>
}
