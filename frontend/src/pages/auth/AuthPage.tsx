import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowRight } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { z } from 'zod'

import { useSession } from '@/features/session'
import { ApiError } from '@/shared/api'
import { Brand, Button, TextField } from '@/shared/ui'
import styles from './AuthPage.module.css'

const authSchema = z.object({
  email: z.email('Введите корректный email'),
  password: z.string(),
})

type AuthForm = z.infer<typeof authSchema>

export function AuthPage({ mode }: { mode: 'login' | 'register' }) {
  const session = useSession()
  const navigate = useNavigate()
  const location = useLocation()
  const [formError, setFormError] = useState<string | null>(null)
  const isRegister = mode === 'register'
  const schema = authSchema.superRefine((values, context) => {
    if (!values.password) context.addIssue({ code: 'custom', path: ['password'], message: 'Введите пароль' })
    if (isRegister && values.password.length > 0 && values.password.length < 10) context.addIssue({ code: 'custom', path: ['password'], message: 'Минимум 10 символов' })
    if (values.password.length > 128) context.addIssue({ code: 'custom', path: ['password'], message: 'Не больше 128 символов' })
  })
  const { register, handleSubmit, formState } = useForm<AuthForm>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', password: '' },
  })

  const onSubmit = handleSubmit(async (values) => {
    setFormError(null)
    try {
      if (isRegister) {
        await session.register(values)
      } else {
        await session.login({ email: values.email, password: values.password })
      }
      const target = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname
      await navigate(target ?? '/vacancies', { replace: true })
    } catch (error) {
      setFormError(error instanceof ApiError && error.status === 401 ? 'Неверный email или пароль' : 'Не удалось войти. Попробуйте ещё раз.')
    }
  })

  return (
    <main className={styles.page}>
      <section className={styles.auth}>
        <Brand className={styles.brand} />
        <div className={styles.formCard}>
          <header>
            <h1>{isRegister ? 'Регистрация' : 'Вход'}</h1>
          </header>
          <form className={styles.form} onSubmit={(event) => void onSubmit(event)} noValidate>
            <TextField label="Email" type="email" placeholder="anna@company.ru" autoComplete="email" error={formState.errors.email?.message} {...register('email')} />
            <TextField label="Пароль" type="password" placeholder="Не меньше 10 символов" autoComplete={isRegister ? 'new-password' : 'current-password'} error={formState.errors.password?.message} {...register('password')} />
            {formError ? <p className={styles.formError} role="alert">{formError}</p> : null}
            <Button variant="highIntent" type="submit" loading={formState.isSubmitting}>
              {isRegister ? 'Создать аккаунт' : 'Войти'} <ArrowRight size={18} />
            </Button>
          </form>
          <p className={styles.switchMode}>
            {isRegister ? 'Уже есть аккаунт?' : 'Ещё нет аккаунта?'}{' '}
            <Link to={isRegister ? '/login' : '/register'}>{isRegister ? 'Войти' : 'Зарегистрироваться'}</Link>
          </p>
        </div>
      </section>
    </main>
  )
}
