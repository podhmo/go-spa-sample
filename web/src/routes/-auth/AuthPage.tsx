import { Link, useRouter } from '@tanstack/react-router'
import { useReducer } from 'react'
import { RequestError, api } from '../../api'
import { AuthForm } from './AuthForm'
import { useMessages } from '../../i18n'
import { cx } from '../../styles/cx'

// /login and /register share this page; only the endpoint, autocomplete
// hints, and cross-link differ. A signed-in visitor is bounced to `next`
// by beforeLoad.
export function AuthPage({ mode, next }: { mode: 'login' | 'register'; next: string | undefined }) {
  const router = useRouter()
  const [pending, setPending] = useReducer((_: boolean, v: boolean) => v, false)
  const [errors, setErrors] = useReducer(
    (
      _: { name?: string; password?: string; form?: string },
      v: { name?: string; password?: string; form?: string },
    ) => v,
    {},
  )
  const m = useMessages()
  return (
    <div>
      <div className={cx('page-head')}>
        <h1>{mode === 'login' ? m.nav.logIn : m.nav.register}</h1>
      </div>
      <AuthForm
        mode={mode}
        errors={errors}
        submitting={pending}
        onSubmit={(name, password) => {
          setPending(true)
          setErrors({})
          void (async () => {
            try {
              if (mode === 'login') await api.login(name, password)
              else await api.register(name, password)
              // The session cookie just landed. invalidate() re-runs
              // this route's beforeLoad, which now sees the session and
              // bounces to `next` — no explicit navigation (a second
              // push would leave a duplicate history entry).
              await router.invalidate()
            } catch (err) {
              // 422 field errors light up their inputs; the credential
              // failure deliberately carries none, so its message fills
              // the form-level slot.
              if (err instanceof RequestError && err.apiError?.fields) {
                setErrors(err.apiError.fields)
              } else {
                setErrors({ form: err instanceof Error ? err.message : String(err) })
              }
            } finally {
              setPending(false)
            }
          })()
        }}
      />
      <p className="muted">
        {mode === 'login' ? (
          <>
            {m.auth.noAccount}{' '}
            <Link to="/register" search={{ next }}>
              {m.nav.register}
            </Link>
          </>
        ) : (
          <>
            {m.auth.alreadyRegistered}{' '}
            <Link to="/login" search={{ next }}>
              {m.nav.logIn}
            </Link>
          </>
        )}
      </p>
    </div>
  )
}
