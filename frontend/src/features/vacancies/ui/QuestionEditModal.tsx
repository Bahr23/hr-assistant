import { Save } from 'lucide-react'
import { useState } from 'react'

import type { components } from '@/shared/api'
import { Button, Modal, SelectField } from '@/shared/ui'
import styles from './QuestionEditModal.module.css'
import { EvaluationGuideField } from './EvaluationGuideField'

type AnswerType = components['schemas']['QuestionAnswerType']

type QuestionEditModalProps = {
  initialEvaluationGuide?: string | null
  initialText: string
  initialAnswerType: AnswerType
  onClose: () => void
  onSave: (text: string, answerType: AnswerType, evaluationGuide: string | null) => void
}

export function QuestionEditModal({ initialText, initialAnswerType, initialEvaluationGuide, onClose, onSave }: QuestionEditModalProps) {
  const [text, setText] = useState(initialText)
  const [answerType, setAnswerType] = useState<AnswerType>(initialAnswerType)
  const [guide, setGuide] = useState(initialEvaluationGuide ?? '')
  const normalizedGuide = guide.trim() || null
  const normalizedText = text.trim()

  return (
    <Modal
      open
      title="Редактировать вопрос"
      description="Измените формулировку, которую увидит кандидат."
      onClose={onClose}
    >
      <form
        className={styles.form}
        onSubmit={(event) => {
          event.preventDefault()
          if (normalizedText) onSave(normalizedText, answerType, normalizedGuide)
        }}
      >
        <label>
          <span>Текст вопроса</span>
          <textarea
            autoFocus
            value={text}
            rows={6}
            maxLength={2000}
            onChange={(event) => setText(event.target.value)}
          />
        </label>
        <SelectField
          label="Формат ответа"
          value={answerType}
          onValueChange={setAnswerType}
          options={[
            { value: 'text', label: 'Текстовый ответ' },
            { value: 'voice', label: 'Голосовое сообщение' },
          ]}
        />
        <EvaluationGuideField value={guide} onChange={setGuide} />
        <div className={styles.actions}>
          <Button type="button" variant="ghost" onClick={onClose}>Отмена</Button>
          <Button
            type="submit"
            variant="brand"
            disabled={!normalizedText || (normalizedText === initialText.trim() && answerType === initialAnswerType && normalizedGuide === (initialEvaluationGuide?.trim() || null))}
          >
            <Save size={18} /> Сохранить
          </Button>
        </div>
      </form>
    </Modal>
  )
}
