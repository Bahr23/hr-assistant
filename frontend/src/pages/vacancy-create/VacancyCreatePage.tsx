import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ArrowRight, Check, GripVertical, Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { Link, useNavigate } from 'react-router-dom'

import { QuestionEditModal, vacanciesApi, vacancyKeys } from '@/features/vacancies'
import { EvaluationGuideField, DescriptionField, CriteriaEditor, criteriaValid } from '@/features/vacancies'
import type { components } from '@/shared/api'
import { moveItem } from '@/shared/lib/moveItem'
import { Button, SelectField, TextField } from '@/shared/ui'
import styles from './VacancyCreatePage.module.css'

type VacancyCreate = components['schemas']['VacancyCreate']
type VacancyDetails = Omit<VacancyCreate, 'interview_language' | 'questions'>
type AnswerType = components['schemas']['QuestionAnswerType']
type DraftQuestion = { id: string; text: string; answer_type: AnswerType; evaluationGuide: string | null }
type DurationOption = '5' | '15' | '30' | '60'

const answerTypeLabels: Record<AnswerType, string> = {
  text: 'Текстовый ответ',
  voice: 'Голосовое сообщение',
}

const required = 'Заполните поле'
const durationOptions = [
  { value: '5', label: '5 минут' },
  { value: '15', label: '15 минут' },
  { value: '30', label: '30 минут' },
  { value: '60', label: '60 минут' },
] as const

export function VacancyCreatePage() {
  const [step, setStep] = useState<1 | 2>(1)
  const [question, setQuestion] = useState('')
  const [questionGuide, setQuestionGuide] = useState('')
  const [questionAnswerType, setQuestionAnswerType] = useState<AnswerType>('text')
  const [questions, setQuestions] = useState<DraftQuestion[]>([])
  const [editingQuestionId, setEditingQuestionId] = useState<string | null>(null)
  const [dragOverQuestionIndex, setDragOverQuestionIndex] = useState<number | null>(null)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { register, handleSubmit, getValues, setValue, control, formState } = useForm<VacancyDetails>({
    defaultValues: { interview_duration_minutes: 15, evaluationCriteria: [], description: "" },
  })
  const interviewDuration = String(useWatch({ control, name: 'interview_duration_minutes' }) ?? 15) as DurationOption
 const criteria = useWatch({ control, name: 'evaluationCriteria' }) ?? []
  const createVacancy = useMutation({
    mutationFn: (body: VacancyCreate) => vacanciesApi.create(body),
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({ queryKey: vacancyKeys.all })
      await navigate(`/vacancies/${created.id}`)
    },
  })

  const addQuestion = () => {
    const value = question.trim()
    if (!value) return
    setQuestions((current) => [...current, {
      id: crypto.randomUUID(), text: value, answer_type: questionAnswerType, evaluationGuide: questionGuide.trim() || null,
    }])
    setQuestion('')
    setQuestionGuide('')
  }

  const create = () => {
    if (questions.length === 0 || questions.some((item) => !item.text.trim()) || !criteriaValid(criteria)) return
    createVacancy.mutate({
      ...getValues(),
      interview_language: 'ru',
      questions: questions.map((item) => ({ text: item.text.trim(), answer_type: item.answer_type, evaluationGuide: item.evaluationGuide })),
    })
  }

  const dropQuestion = (sourceQuestionId: string, targetIndex: number) => {
    const sourceIndex = questions.findIndex((item) => item.id === sourceQuestionId)
    if (sourceIndex < 0) return
    setQuestions((current) => moveItem(current, sourceIndex, targetIndex))
    setDragOverQuestionIndex(null)
  }

  const hasInvalidQuestion = questions.some((item) => !item.text.trim())
  const editingQuestion = questions.find((item) => item.id === editingQuestionId)

  return (
    <main className={styles.page}>
      <Link className={styles.back} to="/vacancies"><ArrowLeft /> Все вакансии</Link>

      {step === 1 ? (
        <form className={styles.card} onSubmit={(event) => void handleSubmit(() => setStep(2))(event)}>
          <div className={styles.sectionTitle}><span>1/2</span><div><h2>Расскажите о вакансии</h2><p>Добавьте приветствие, описание позиции и критерии оценки.</p></div></div>
          <div className={styles.twoColumns}>
            <TextField label="Название вакансии" placeholder="Менеджер по продажам" error={formState.errors.title?.message} maxLength={255} {...register('title', { required })} />
            <SelectField
              label="Время на прохождение"
              value={interviewDuration}
              onValueChange={(value) => setValue('interview_duration_minutes', Number(value) as 5 | 15 | 30 | 60, { shouldDirty: true })}
              options={durationOptions}
            />
          </div>
          <label className={styles.textarea}><span>Приветствие кандидата</span><textarea rows={4} maxLength={10000} placeholder="Помогите кандидату настроиться" {...register('welcome_message', { required })} />{formState.errors.welcome_message ? <small>{required}</small> : null}</label>
          <DescriptionField {...register('description', { maxLength: 10000 })} />
          <CriteriaEditor value={criteria} onChange={(value) => setValue('evaluationCriteria', value, { shouldDirty: true })} />
          <div className={styles.formActions}><Button variant="brand" type="submit" disabled={!criteriaValid(criteria)}>Продолжить <ArrowRight size={18} /></Button></div>
        </form>
      ) : (
        <section className={styles.card}>
          <div className={styles.sectionTitle}><span>2/2</span><div><h2>Вопросы</h2><p>Добавьте хотя бы один вопрос в порядке прохождения интервью.</p></div></div>
          <ol className={styles.questions}>
            {questions.map((item, index) => (
              <li
                key={item.id}
                className={dragOverQuestionIndex === index ? styles.dragOver : undefined}
                onDragOver={(event) => {
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
                  draggable={questions.length > 1}
                  aria-label={`Переместить вопрос ${index + 1}`}
                  onDragStart={(event) => {
                    event.dataTransfer.effectAllowed = 'move'
                    event.dataTransfer.setData('text/plain', item.id)
                  }}
                >
                  <GripVertical aria-hidden="true" />
                </button>
                <span>{String(index + 1).padStart(2, '0')}</span>
                <div className={styles.questionContent}>
                  <p>{item.text}</p>
                  <small>{answerTypeLabels[item.answer_type]}</small>
                </div>
                <div className={styles.questionActions}>
                  <button type="button" onClick={() => setEditingQuestionId(item.id)} aria-label={`Редактировать вопрос ${index + 1}`}><Pencil /></button>
                  <button type="button" onClick={() => setQuestions((current) => current.filter((currentItem) => currentItem.id !== item.id))} aria-label={`Удалить вопрос ${index + 1}`}><Trash2 /></button>
                </div>
              </li>
            ))}
          </ol>
          <form className={styles.addQuestion} onSubmit={(event) => { event.preventDefault(); addQuestion() }}>
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
            <EvaluationGuideField value={questionGuide} onChange={setQuestionGuide} />
            <Button type="submit" disabled={!question.trim()}><Plus size={18} /> Добавить вопрос</Button>
          </form>
          {createVacancy.isError ? <p className={styles.apiError} role="alert">Не удалось создать вакансию. Проверьте заполненные данные и попробуйте снова.</p> : null}
          <div className={styles.formActions}>
            <Button variant="ghost" onClick={() => setStep(1)}><ArrowLeft size={18} /> Назад</Button>
            <Button variant="brand" onClick={create} loading={createVacancy.isPending} disabled={questions.length === 0 || hasInvalidQuestion || !criteriaValid(criteria)}><Check size={18} /> Создать</Button>
          </div>
        </section>
      )}
      {editingQuestion ? (
        <QuestionEditModal
          key={editingQuestion.id}
          initialEvaluationGuide={editingQuestion.evaluationGuide}
          initialText={editingQuestion.text}
          initialAnswerType={editingQuestion.answer_type}
          onClose={() => setEditingQuestionId(null)}
          onSave={(text, answerType, evaluationGuide) => {
            setQuestions((current) => current.map((item) => item.id === editingQuestion.id
              ? { ...item, text, answer_type: answerType, evaluationGuide }
              : item))
            setEditingQuestionId(null)
          }}
        />
      ) : null}
    </main>
  )
}
