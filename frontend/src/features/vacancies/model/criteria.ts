import type { components } from '@/shared/api'

export type Criterion = components['schemas']['EvaluationCriterion']
export function criteriaValid(values: Criterion[]) {
  return values.length <= 20 && values.every((c) => c.title.trim().length > 0 && Array.from(c.title.trim()).length <= 120 && c.evaluationGuide.trim().length > 0 && Array.from(c.evaluationGuide.trim()).length <= 2000)
}

