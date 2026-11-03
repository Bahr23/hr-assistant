import { createBrowserRouter, Navigate } from 'react-router-dom'

import { AnonymousRoute, ProtectedRoute } from '@/app/router/RouteGuards'
import { AppShell } from '@/widgets/AppShell/AppShell'
import { AuthPage } from '@/pages/auth/AuthPage'
import { CandidateDetailPage } from '@/pages/candidate-detail/CandidateDetailPage'
import { CandidatesPage } from '@/pages/candidates/CandidatesPage'
import { NotFoundPage } from '@/pages/not-found/NotFoundPage'
import { SettingsPage } from '@/pages/settings/SettingsPage'
import { VacanciesPage } from '@/pages/vacancies/VacanciesPage'
import { VacancyCreatePage } from '@/pages/vacancy-create/VacancyCreatePage'
import { VacancyEditorPage } from '@/pages/vacancy-editor/VacancyEditorPage'

export const router = createBrowserRouter([
  {
    element: <AnonymousRoute />,
    children: [
      { path: '/login', element: <AuthPage mode="login" /> },
      { path: '/register', element: <AuthPage mode="register" /> },
    ],
  },
  {
    element: <ProtectedRoute />,
    children: [
      {
        element: <AppShell />,
        children: [
          { index: true, element: <Navigate to="/vacancies" replace /> },
          { path: '/vacancies', element: <VacanciesPage /> },
          { path: '/vacancies/new', element: <VacancyCreatePage /> },
          { path: '/vacancies/:vacancyId', element: <VacancyEditorPage /> },
          { path: '/candidates', element: <CandidatesPage /> },
          { path: '/candidates/:sessionId', element: <CandidateDetailPage /> },
          { path: '/settings', element: <SettingsPage /> },
        ],
      },
    ],
  },
  { path: '*', element: <NotFoundPage /> },
])
