import { useReducer } from 'react'
import { useMessages } from '../../i18n'
import { cx } from '../../styles/cx'

// Register/login share one form: name + password, ?next handled by the
// caller. mode picks labels and the password autocomplete token.
export function AuthForm({
  mode,
  errors,
  submitting,
  onSubmit,
}: {
  mode: 'login' | 'register'
  errors?: { name?: string; password?: string; form?: string } | undefined
  submitting?: boolean
  onSubmit: (name: string, password: string) => void
}) {
  const [state, dispatch] = useReducer(
    (s: { name: string; password: string }, a: { field: 'name' | 'password'; value: string }) => ({
      ...s,
      [a.field]: a.value,
    }),
    { name: '', password: '' },
  )
  const id = mode === 'login' ? 'login' : 'register'
  const m = useMessages()
  return (
    <form
      className={cx('card')}
      aria-label={mode === 'login' ? m.auth.logInAria : m.auth.registerAria}
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(state.name, state.password)
      }}
    >
      <p className={cx('form-note')}>
        {m.form.requiredPre}{' '}
        <span className={cx('req')} aria-hidden="true">
          *
        </span>{' '}
        {m.form.requiredPost}
      </p>
      {errors?.form && (
        <em className={cx('error-msg')} role="alert">
          {errors.form}
        </em>
      )}
      <div className={cx('field')}>
        <label htmlFor={`${id}-name`}>
          {m.form.name}{' '}
          <span className={cx('req')} aria-hidden="true">
            *
          </span>
        </label>
        <input
          type="text"
          id={`${id}-name`}
          name="name"
          value={state.name}
          onChange={(e) => dispatch({ field: 'name', value: e.target.value })}
          required
          // Login names are identifiers, not prose — keep the browser
          // from rewriting them (autofill-sign-in-form guidance).
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="none"
          autoComplete="username"
          data-error={`${id}-name-error`}
          aria-invalid={errors?.name ? 'true' : 'false'}
          aria-describedby={errors?.name ? `${id}-name-api-error` : undefined}
        />
        <span id={`${id}-name-error`} className={cx('error-msg')}>
          {m.form.nameRequired}
        </span>
        {errors?.name && (
          <em className={cx('error-msg')} id={`${id}-name-api-error`} role="alert">
            {errors.name}
          </em>
        )}
      </div>
      <div className={cx('field')}>
        <label htmlFor={`${id}-password`}>
          {m.form.password}{' '}
          <span className={cx('req')} aria-hidden="true">
            *
          </span>
        </label>
        <input
          type="password"
          id={`${id}-password`}
          name="password"
          value={state.password}
          onChange={(e) => dispatch({ field: 'password', value: e.target.value })}
          required
          {...(mode === 'register' ? { minLength: 8 } : {})}
          autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
          data-error={`${id}-password-error`}
          aria-invalid={errors?.password ? 'true' : 'false'}
          aria-describedby={errors?.password ? `${id}-password-api-error` : undefined}
        />
        <span id={`${id}-password-error`} className={cx('error-msg')}>
          {mode === 'register' ? m.form.passwordMin : m.form.passwordRequired}
        </span>
        {errors?.password && (
          <em className={cx('error-msg')} id={`${id}-password-api-error`} role="alert">
            {errors.password}
          </em>
        )}
      </div>
      <div className={cx('form-actions')}>
        <button type="submit" className={cx('btn btn-primary')} disabled={submitting}>
          {submitting ? '…' : mode === 'login' ? m.nav.logIn : m.nav.register}
        </button>
      </div>
    </form>
  )
}
