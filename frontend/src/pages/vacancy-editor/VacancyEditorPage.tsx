import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Archive, ArrowLeft, Check, Copy, GripVertical, Pencil, Plus, Save, Trash2, Undo2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { Link, useBlocker, useParams } from 'react-router-dom'

import { VacancyStatusBadge } from '@/entities/vacancy'
import { DescriptionField, CriteriaEditor, criteriaValid, EvaluationGuideField, QuestionEditModal, vacanciesApi, vacancyDetailQuery, vacancyKeys } from '@/features/vacancies'
import type { components } from '@/shared/api'
import { moveItem } from '@/shared/lib/moveItem'
import { Button, Modal, SelectField, Spinner, TextField } from '@/shared/ui'
import styles from './VacancyEditorPage.module.css'
import { vacancyErrorMessage } from './vacancyErrorMessage'

type VacancyPatch = components['schemas']['VacancyPatch']
type Question = components['schemas']['Question']
type AnswerType = components['schemas']['QuestionAnswerType']
type DurationOption = '5' | '15' | '30' | '60'
type Action = 'archive' | 'restore'
type QuestionSavePayload = {
  updates: Array<{ id: string; text: string; answerType: AnswerType; evaluationGuide: string | null }>
  order: string[] | null
}
type EditorSavePayload = {
  vacancy: VacancyPatch
  questions?: QuestionSavePayload
}

const answerTypeLabels: Record<AnswerType, string> = {
  text: 'Текстовый ответ',
  voice: 'Голосовое сообщение',
}

const durationOptions = [
  { value: '5', label: '5 минут' },
  { value: '15', label: '15 минут' },
  { value: '30', label: '30 минут' },
  { value: '60', label: '60 минут' },
] as const

export function VacancyEditorPage() {
  const vacancyId = useParams().vacancyId ?? ''
  const queryClient = useQueryClient()
  const vacancy = useQuery(vacancyDetailQuery(vacancyId))
  const [question, setQuestion] = useState('')
  const [questionGuide, setQuestionGuide] = useState('')
  const [questionGuideDrafts, setQuestionGuideDrafts] = useState<Record<string, string | null>>({})
  const [questionAnswerType, setQuestionAnswerType] = useState<AnswerType>('text')
  const [draftQuestionOrder, setDraftQuestionOrder] = useState<string[] | null>(null)
  const [questionTextDrafts, setQuestionTextDrafts] = useState<Record<string, string>>({})
  const [questionTypeDrafts, setQuestionTypeDrafts] = useState<Record<string, AnswerType>>({})
  const [editingQuestionId, setEditingQuestionId] = useState<string | null>(null)
  const [dragOverQuestionIndex, setDragOverQuestionIndex] = useState<number | null>(null)
  const [showSavedState, setShowSavedState] = useState(false)
  const { register, handleSubmit, reset, setValue, control, formState } = useForm<VacancyPatch>()
  const interviewDuration = String(useWatch({ control, name: 'interview_duration_minutes' }) ?? 15) as DurationOption
 const criteria = useWatch({ control, name: 'evaluationCriteria' }) ?? []

  useEffect(() => {
    if (!vacancy.data || formState.isDirty) return
    reset({
      title: vacancy.data.title,
 description: vacancy.data.description ?? "",
 evaluationCriteria: vacancy.data.evaluationCriteria ?? [],
      welcome_message: vacancy.data.welcome_message,
      interview_duration_minutes: vacancy.data.interview_duration_minutes,
    })
  }, [formState.isDirty, vacancy.data, reset])

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: vacancyKeys.detail(vacancyId) })
    await queryClient.invalidateQueries({ queryKey: vacancyKeys.all })
  }

  const addQuestion = useMutation({
    mutationFn: ({ text, answerType, evaluationGuide }: { text: string; answerType: AnswerType; evaluationGuide: string | null }) => (
      vacanciesApi.addQuestion(vacancyId, text, answerType, evaluationGuide)
    ),
    onSuccess: async () => {
      setQuestion('')
      setQuestionGuide('')
      await refresh()
    },
  })

  const deleteQuestion = useMutation({
    mutationFn: (questionId: string) => vacanciesApi.deleteQuestion(vacancyId, questionId),
    onSuccess: refresh,
  })

  const saveChanges = useMutation({
    mutationFn: async ({ vacancy: vacancyChanges, questions: questionChanges }: EditorSavePayload) => {
      await vacanciesApi.update(vacancyId, vacancyChanges)
      await Promise.all([
        ...(questionChanges?.updates.map((item) => (
          vacanciesApi.updateQuestion(vacancyId, item.id, item.text, item.answerType, item.evaluationGuide)
        )) ?? []),
        ...(questionChanges?.order ? [vacanciesApi.reorderQuestions(vacancyId, questionChanges.order)] : []),
      ])
    },
    onSuccess: async (_, variables) => {
      reset(variables.vacancy)
      await refresh()
      setDraftQuestionOrder(null)
      setQuestionTextDrafts({})
      setQuestionTypeDrafts({})
      setQuestionGuideDrafts({})
      setShowSavedState(true)
    },
  })

  const lifecycle = useMutation({
    mutationFn: (action: Action) => vacanciesApi.action(vacancyId, action),
    onSuccess: refresh,
  })

  const data = vacancy.data
  const serverQuestions = data?.questions ?? []
  const questionById = new Map(serverQuestions.map((item) => [item.id, item]))
  const serverQuestionIds = serverQuestions.map((item) => item.id)
  const orderedQuestionIds = draftQuestionOrder
    ? [
        ...draftQuestionOrder.filter((id) => questionById.has(id)),
        ...serverQuestionIds.filter((id) => !draftQuestionOrder.includes(id)),
      ]
    : serverQuestionIds
  const questions = orderedQuestionIds
    .map((id) => questionById.get(id))
    .filter((item): item is Question => item !== undefined)
    .map((item) => ({
      ...item,
      text: questionTextDrafts[item.id] ?? item.text,
      answer_type: questionTypeDrafts[item.id] ?? item.answer_type,
      evaluationGuide: item.id in questionGuideDrafts ? (questionGuideDrafts[item.id] ?? null) : item.evaluationGuide,
    }))
  const questionUpdates = serverQuestions.flatMap((item) => {
    const text = questionTextDrafts[item.id]?.trim() ?? item.text
    const answerType = questionTypeDrafts[item.id] ?? item.answer_type
    const evaluationGuide = item.id in questionGuideDrafts ? (questionGuideDrafts[item.id] ?? null) : item.evaluationGuide
    if (text === item.text && answerType === item.answer_type && evaluationGuide === item.evaluationGuide) return []
    return [{ id: item.id, text, answerType, evaluationGuide }]
  })
  const questionOrderChanged = orderedQuestionIds.some((id, index) => id !== serverQuestionIds[index])
  const questionsDirty = questionUpdates.length > 0 || questionOrderChanged
  const hasInvalidQuestion = questions.some((item) => !item.text.trim())
  const editingQuestion = questions.find((item) => item.id === editingQuestionId)
  const isArchived = data?.status === 'archived'
  const hasUnsavedChanges = formState.isDirty || questionsDirty
  const blocker = useBlocker(hasUnsavedChanges)

  useEffect(() => {
    if (!hasUnsavedChanges) return

    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }

    window.addEventListener('beforeunload', warnBeforeUnload)
    return () => window.removeEventListener('beforeunload', warnBeforeUnload)
  }, [hasUnsavedChanges])

  if (vacancy.isPending) return <Spinner label="Открываем описание интервью…" />
  if (vacancy.isError || !data) {
    return <main className={styles.error}><h1>Вакансия не найдена</h1><p>Возможно, она была удалена или принадлежит другому пользователю.</p><Link to="/vacancies">К списку вакансий</Link></main>
  }

  const error = saveChanges.error ?? addQuestion.error ?? deleteQuestion.error ?? lifecycle.error
  const errorMessage = vacancyErrorMessage(error)

  const copyLink = async () => {
    if (!data.interview_url) return
    await navigator.clipboard.writeText(data.interview_url)
  }

  const dropQuestion = (sourceQuestionId: string, targetIndex: number) => {
    const sourceIndex = questions.findIndex((item) => item.id === sourceQuestionId)
    if (sourceIndex < 0) return
    const reordered = moveItem(questions, sourceIndex, targetIndex)
    const questionIds = reordered.map((item) => item.id)
    setDraftQuestionOrder(questionIds)
    setDragOverQuestionIndex(null)
  }

  const persistChanges = (afterSave?: () => void) => {
 if (!criteriaValid(criteria)) return
    void handleSubmit((values) => saveChanges.mutate({
      vacancy: { ...values, interview_language: 'ru' },
      questions: questionsDirty ? {
        updates: questionUpdates,
        order: questionOrderChanged ? orderedQuestionIds : null,
      } : undefined,
    }, { onSuccess: afterSave }))()
  }

  return (
    <main className={styles.page}>
      <div className={styles.topbar}>
        <Link className={styles.back} to="/vacancies"><ArrowLeft /> Все вакансии</Link>
      </div>

      <header className={styles.header}>
        <div>
          <VacancyStatusBadge status={data.status} />
          <div className={styles.titleLine}>
            <h1>{data.title || 'Вакансия без названия'}</h1>
          </div>
        </div>
        <div className={styles.headerControls}>
          {data.status === 'archived' ? (
            <div className={styles.lifecycleActions}>
              <Button variant="brand" onClick={() => lifecycle.mutate('restore')} loading={lifecycle.isPending}><Undo2 size={18} /> Активировать</Button>
            </div>
          ) : null}
          {data.interview_url ? (
            <section className={styles.publicationBar} aria-label="Управление публичной ссылкой">
              <a className={styles.linkValue} href={data.interview_url} target="_blank" rel="noreferrer">{data.interview_url}</a>
              <button className={styles.copyButton} type="button" onClick={() => void copyLink()} aria-label="Скопировать ссылку" title="Скопировать ссылку"><Copy aria-hidden="true" /></button>
              <Button className={styles.publicationAction} variant="ghost" size="sm" onClick={() => lifecycle.mutate('archive')} loading={lifecycle.isPending}><Archive size={16} /> Архивировать</Button>
            </section>
          ) : null}
        </div>
      </header>

      {errorMessage ? <p className={styles.apiError} role="alert">{errorMessage}</p> : null}

      <div className={styles.layout}>
        <form className={styles.form} onSubmit={(event) => { event.preventDefault(); persistChanges() }}>
          <section className={styles.section}>
            <div className={styles.sectionTitle}><span>01</span><div><h2>О вакансии</h2><p>То, что кандидат увидит перед началом интервью.</p></div></div>
            <div className={styles.twoColumns}>
              <TextField label="Название вакансии" placeholder="Менеджер по продажам" disabled={isArchived} maxLength={255} {...register('title')} />
              <SelectField
                label="Время на прохождение"
                value={interviewDuration}
                disabled={isArchived}
                onValueChange={(value) => setValue('interview_duration_minutes', Number(value) as 5 | 15 | 30 | 60, { shouldDirty: true })}
                options={durationOptions}
              />
            </div>
            <label className={styles.textarea}><span>Приветствие кандидата</span><textarea rows={4} maxLength={10000} placeholder="Помогите кандидату настроиться" disabled={isArchived} {...register('welcome_message')} /></label>
            <DescriptionField disabled={isArchived} {...register('description', { maxLength: 10000 })} />
          </section>
          <section className={styles.section}><CriteriaEditor value={criteria} onChange={(value) => setValue('evaluationCriteria', value, { shouldDirty: true })} disabled={isArchived || saveChanges.isPending} /></section>
        </form>

        <aside className={styles.aside}>
          <section className={styles.section}>
            <div className={styles.sectionTitle}><span>02</span><div><h2>Вопросы</h2><p>{questions.length} {questions.length === 1 ? 'вопрос' : 'вопросов'} в интервью</p></div></div>
            <ol className={styles.questions}>
              {questions.map((item, index) => (
                <li
                  key={item.id}
                  className={dragOverQuestionIndex === index ? styles.dragOver : undefined}
                  onDragOver={(event) => {
                    if (isArchived) return
                    event.preventDefault()
                    event.dataTransfer.dropEffect = 'move'
                    setDragOverQuestionIndex(index)
                  }}
                  onDrop={(event) => {
                    event.preventDefault()
                    dropQuestion(event.dataTransfer.getData('text/plain'), index)
                  }}
                  onDragEnd={() => {
                    setDragOverQuestionIndex(null)
                  }}
                >
                  <button
                    className={styles.dragHandle}
                    type="button"
                    draggable={!isArchived && questions.length > 1 && !saveChanges.isPending}
                    disabled={isArchived}
                    aria-label={`Переместить вопрос ${index + 1}`}
                    onDragStart={(event) => {
                      event.dataTransfer.effectAllowed = 'move'
                      event.dataTransfer.setData('text/plain', item.id)
                    }}
                  >
                    <GripVertical aria-hidden="true" />
                  </button>
                  <span className={styles.questionNumber}>{String(index + 1).padStart(2, '0')}</span>
                  <div className={styles.questionContent}>
                    <p>{item.text}</p>
                    <small>{answerTypeLabels[item.answer_type]}</small>
                  </div>
                  {!isArchived ? (
                    <div className={styles.questionActions}>
                      <button type="button" onClick={() => setEditingQuestionId(item.id)} aria-label={`Редактировать вопрос ${index + 1}`}><Pencil /></button>
                      <button type="button" onClick={() => deleteQuestion.mutate(item.id)} aria-label={`Удалить вопрос ${index + 1}`}><Trash2 /></button>
                    </div>
                  ) : null}
                </li>
              ))}
            </ol>
            {!isArchived ? (
              <form className={styles.addQuestion} onSubmit={(event) => {
                event.preventDefault()
                if (question.trim()) addQuestion.mutate({ text: question.trim(), answerType: questionAnswerType, evaluationGuide: questionGuide.trim() || null })
              }}>
                <textarea value={question} onChange={(event) => setQuestion(event.target.value)} rows={3} maxLength={2000} placeholder="Например: расскажите о вашем самом сложном рабочем кейсе" />
                <SelectField
                  label="Формат ответа"
                  value={questionAnswerType}
                  onValueChange={setQuestionAnswerType}
                  options={[
                    { value: 'text', label: 'Текстовый ответ' },
                    { value: 'voice', label: 'Голосовое сообщение' },
                  ]}
                />
                <EvaluationGuideField value={questionGuide} onChange={setQuestionGuide} disabled={addQuestion.isPending} />
                <Button type="submit" loading={addQuestion.isPending} disabled={!question.trim()}><Plus size={18} /> Добавить вопрос</Button>
              </form>
            ) : null}
          </section>

        </aside>
      </div>
      {!isArchived && hasUnsavedChanges ? (
          <Button
            className={styles.floatingSave}
            variant="brand"
            loading={saveChanges.isPending}
            disabled={hasInvalidQuestion || !criteriaValid(criteria)}
            onClick={() => persistChanges()}
            aria-label="Сохранить изменения"
            title="Сохранить изменения"
          >
            {!saveChanges.isPending ? <Save aria-hidden="true" /> : null}
          </Button>
        ) : null}
      {!isArchived && !hasUnsavedChanges && showSavedState ? (
        <div className={`${styles.floatingSave} ${styles.floatingSaved}`} role="status" aria-label="Изменения сохранены" title="Изменения сохранены">
          <Check aria-hidden="true" />
        </div>
      ) : null}
      {editingQuestion ? (
        <QuestionEditModal
          key={editingQuestion.id}
          initialEvaluationGuide={editingQuestion.evaluationGuide}
          initialText={editingQuestion.text}
          initialAnswerType={editingQuestion.answer_type}
          onClose={() => setEditingQuestionId(null)}
          onSave={(text, answerType, evaluationGuide) => {
            setQuestionGuideDrafts((current) => ({ ...current, [editingQuestion.id]: evaluationGuide }))
            setQuestionTextDrafts((current) => ({ ...current, [editingQuestion.id]: text }))
            setQuestionTypeDrafts((current) => ({ ...current, [editingQuestion.id]: answerType }))
            setEditingQuestionId(null)
          }}
        />
      ) : null}
      {blocker.state === 'blocked' ? (
        <Modal
          open
          title="Есть несохранённые изменения"
          description="Сохраните изменения перед выходом или покиньте редактор без сохранения."
          onClose={() => blocker.reset()}
        >
          <div className={styles.leaveDialogActions}>
            <Button type="button" variant="ghost" onClick={() => blocker.reset()}>Отмена</Button>
            <Button type="button" variant="danger" onClick={() => blocker.proceed()}>Выйти без сохранения</Button>
            <Button
              type="button"
              variant="brand"
              loading={saveChanges.isPending}
              disabled={hasInvalidQuestion || !criteriaValid(criteria)}
              onClick={() => persistChanges(() => blocker.proceed())}
            >
              <Save size={18} /> Сохранить и выйти
            </Button>
          </div>
        </Modal>
      ) : null}
    </main>
  )
}
