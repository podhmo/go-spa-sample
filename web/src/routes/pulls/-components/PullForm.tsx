import { useReducer, type ReactNode } from 'react'
import type { PullRef } from '../../../api'
import { submitOnCtrlEnter } from '../../../enter-submit'
import { useMessages } from '../../../i18n'
import styles from './pulls.module.css'
import { useLeaveGuard } from '../../../leave-guard'
import { cx } from '../../../styles/cx'

type PullFormState = {
  title: string
  body: string
  base: string
  head: string
  submitted: boolean
}
type PullFormAction =
  | { type: 'field'; field: 'title' | 'body' | 'base' | 'head'; value: string }
  | { type: 'submitted' }

const pullFormReducer = (s: PullFormState, a: PullFormAction): PullFormState => {
  switch (a.type) {
    case 'field':
      return { ...s, [a.field]: a.value, submitted: false }
    case 'submitted':
      // The leave-guard must release before the parent navigates away.
      return { ...s, submitted: true }
  }
}

// New/edit pull request. Creation needs base+head (branch refs from
// /api/v1/pulls/refs); editing touches title/body only, matching the
// PATCH endpoint.
export function PullForm({
  onSubmit,
  submitting,
  errors,
  initial,
  refs,
  submitLabel,
  cancelLink,
}: {
  onSubmit: (input: { title: string; body: string; base?: string; head?: string }) => void
  submitting?: boolean
  errors?: Record<string, string>
  initial?: { title: string; body: string }
  refs?: PullRef[]
  submitLabel?: string
  cancelLink?: ReactNode
}) {
  const editing = initial !== undefined
  const init = { title: initial?.title ?? '', body: initial?.body ?? '' }
  const [state, dispatch] = useReducer(pullFormReducer, {
    title: init.title,
    body: init.body,
    base: '',
    head: '',
    submitted: false,
  })
  const branches = (refs ?? []).filter((r) => r.name.startsWith('refs/heads/'))
  // Head may be a remote-tracking ref too (createPull accepts any ref);
  // base must stay a local branch because merging advances it.
  const headRefs = (refs ?? []).filter((r) => !r.name.startsWith('refs/tags/'))
  const m = useMessages()
  useLeaveGuard(
    !state.submitted &&
      (state.title !== init.title ||
        state.body !== init.body ||
        state.base !== '' ||
        state.head !== ''),
    m.form.unsavedChanges,
  )
  return (
    <form
      className={cx('card issue-form')}
      aria-label={editing ? m.pulls.editAria : m.pulls.newAria}
      onSubmit={(e) => {
        e.preventDefault()
        dispatch({ type: 'submitted' })
        const { submitted: _submitted, ...input } = state
        onSubmit(editing ? { title: input.title, body: input.body } : input)
      }}
    >
      <p className={cx('form-note')}>
        {m.form.requiredPre}{' '}
        <span className={cx('req')} aria-hidden="true">
          *
        </span>{' '}
        {m.form.requiredPost}
      </p>
      {!editing && (
        <div className={styles.pullRefsRow}>
          <div className={cx('field')}>
            <label htmlFor="pull-head">
              {m.pulls.head}{' '}
              <span className={cx('req')} aria-hidden="true">
                *
              </span>
            </label>
            <select
              id="pull-head"
              name="head"
              value={state.head}
              onChange={(e) => dispatch({ type: 'field', field: 'head', value: e.target.value })}
              required
              data-error="pull-head-error"
              aria-invalid={errors?.head ? 'true' : 'false'}
              aria-describedby={errors?.head ? 'pull-head-api-error' : undefined}
            >
              <option value="" disabled>
                {m.pulls.headPh}
              </option>
              {headRefs.map((r) => (
                <option key={r.name} value={r.name}>
                  {r.short}
                </option>
              ))}
            </select>
            <span id="pull-head-error" className={cx('error-msg')}>
              {m.pulls.headRequired}
            </span>
            {errors?.head && (
              <em id="pull-head-api-error" role="alert">
                {errors.head}
              </em>
            )}
          </div>
          <div className={cx('field')}>
            <label htmlFor="pull-base">
              {m.pulls.base}{' '}
              <span className={cx('req')} aria-hidden="true">
                *
              </span>
            </label>
            <select
              id="pull-base"
              name="base"
              value={state.base}
              onChange={(e) => dispatch({ type: 'field', field: 'base', value: e.target.value })}
              required
              data-error="pull-base-error"
              aria-invalid={errors?.base ? 'true' : 'false'}
              aria-describedby={errors?.base ? 'pull-base-api-error' : undefined}
            >
              <option value="" disabled>
                {m.pulls.basePh}
              </option>
              {branches.map((r) => (
                <option key={r.name} value={r.name}>
                  {r.short}
                </option>
              ))}
            </select>
            <span id="pull-base-error" className={cx('error-msg')}>
              {m.pulls.baseRequired}
            </span>
            {errors?.base && (
              <em id="pull-base-api-error" role="alert">
                {errors.base}
              </em>
            )}
          </div>
        </div>
      )}
      <div className={cx('field')}>
        <label htmlFor="pull-title">
          {m.form.title}{' '}
          <span className={cx('req')} aria-hidden="true">
            *
          </span>
        </label>
        <input
          type="text"
          id="pull-title"
          name="title"
          value={state.title}
          onChange={(e) => dispatch({ type: 'field', field: 'title', value: e.target.value })}
          required
          data-error="pull-title-error"
          aria-invalid={errors?.title ? 'true' : 'false'}
          aria-describedby={errors?.title ? 'pull-title-api-error' : undefined}
        />
        <span id="pull-title-error" className={cx('error-msg')}>
          {m.form.titleRequired}
        </span>
        {errors?.title && (
          <em id="pull-title-api-error" role="alert">
            {errors.title}
          </em>
        )}
      </div>
      <div className={cx('field')}>
        <label htmlFor="pull-body">{m.form.body}</label>
        <textarea
          id="pull-body"
          name="body"
          value={state.body}
          onChange={(e) => dispatch({ type: 'field', field: 'body', value: e.target.value })}
          onKeyDown={submitOnCtrlEnter}
          rows={8}
          aria-invalid={errors?.body ? 'true' : 'false'}
          aria-describedby={errors?.body ? 'pull-body-api-error' : undefined}
        />
        {errors?.body && (
          <em id="pull-body-api-error" role="alert">
            {errors.body}
          </em>
        )}
      </div>
      <div className={cx('form-actions')}>
        {cancelLink}
        <button type="submit" className={cx('btn btn-primary')} disabled={submitting}>
          {submitting
            ? m.form.saving
            : (submitLabel ?? (editing ? m.form.saveChanges : m.pulls.create))}
        </button>
      </div>
    </form>
  )
}
