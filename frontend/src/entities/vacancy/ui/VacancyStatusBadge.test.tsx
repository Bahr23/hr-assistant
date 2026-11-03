import { render, screen } from '@testing-library/react'

import { VacancyStatusBadge } from './VacancyStatusBadge'

describe('VacancyStatusBadge', () => {
  it.each([
    ['published', 'Активна'],
    ['archived', 'Архив'],
  ] as const)('renders %s label', (status, label) => {
    render(<VacancyStatusBadge status={status} />)
    expect(screen.getByText(label)).toBeVisible()
  })
})
