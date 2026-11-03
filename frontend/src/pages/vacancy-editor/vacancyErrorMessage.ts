import { ApiError } from '@/shared/api'

const publicationFieldLabels: Record<string, string> = {
  title: 'Название вакансии',
  interview_language: 'Язык интервью',
  welcome_message: 'Приветствие кандидата',
  interview_duration_minutes: 'Время на прохождение',
}

export function vacancyErrorMessage(error: unknown) {
  if (!(error instanceof ApiError)) {
    return error ? 'Не удалось выполнить действие' : null
  }

  if (error.code === 'publication_incomplete') {
    if (error.field === 'questions') return 'Добавьте хотя бы один вопрос для интервью.'
    if (error.field === 'expires_at') return 'У вакансии истёк срок приёма ответов.'

    const label = error.field ? publicationFieldLabels[error.field] : undefined
    return label ? `Заполните поле «${label}».` : 'Заполните обязательные поля перед публикацией.'
  }

  return error.message
}
