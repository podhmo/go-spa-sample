import { Link } from '@tanstack/react-router'
import { useReducer, type ReactNode } from 'react'
import type { IssueInput, Label } from '../../../../../api'
import { submitOnCtrlEnter } from '../../../../../enter-submit'
import { useMessages } from '../../../../../i18n'
import { useLeaveGuard } from '../../../../../leave-guard'
import { LabelBadge } from '../../../../../components/LabelBadge'
import styles from './IssueForm.module.css'
import { cx } from '../../../../../styles/cx'

type IssueFormState = { title: string; body: string; label_ids: number[]; submitted: boolean }
type IssueFormAction =
  | { type: 'field'; field: 'title' | 'body'; value: string }
  | { type: 'toggle-label'; id: number }
  | { type: 'submitted' }

const issueFormReducer = (s: IssueFormState, a: IssueFormAction): IssueFormState => {
  switch (a.type) {
    case 'field':
      return { ...s, [a.field]: a.value, submitted: false }
    case 'toggle-label':
      return {
        ...s,
        submitted: false,
        label_ids: s.label_ids.includes(a.id)
          ? s.label_ids.filter((id) => id !== a.id)
          : [...s.label_ids, a.id],
      }
    case 'submitted':
      // The leave-guard must release before the parent navigates away.
      return { ...s, submitted: true }
  }
}

export function IssueForm({
  onSubmit,
  submitting,
  errors,
  initial,
  labels,
  submitLabel,
  cancelLink,
}: {
  onSubmit: (input: IssueInput) => void
  submitting?: boolean
  errors?: Record<string, string>
  initial?: { title: string; body: string; label_ids?: number[] }
  // Selectable labels (active, kind=issue). Archived labels already
  // attached to the issue are absent here and preserved server-side.
  labels?: Label[]
  submitLabel?: string
  // The left-hand Cancel action (a <Link> back to where the user came
  // from; the MPA posts forms to their own URLs — the SPA navigates).
  cancelLink?: ReactNode
}) {
  const init = {
    title: initial?.title ?? '',
    body: initial?.body ?? '',
    label_ids: initial?.label_ids ?? [],
  }
  const [state, dispatch] = useReducer(issueFormReducer, { ...init, submitted: false })
  const m = useMessages()
  useLeaveGuard(
    !state.submitted &&
      (state.title !== init.title ||
        state.body !== init.body ||
        state.label_ids.length !== init.label_ids.length ||
        state.label_ids.some((id) => !init.label_ids.includes(id))),
    m.form.unsavedChanges,
  )
  return (
    <form
      className={cx('card issue-form')}
      aria-label={initial ? m.issues.editAria : m.issues.newAria}
      onSubmit={(e) => {
        e.preventDefault()
        dispatch({ type: 'submitted' })
        const { submitted: _submitted, ...input } = state
        onSubmit(input)
      }}
    >
      <p className={cx('form-note')}>
        {m.form.requiredPre} <span className={cx('req')}>*</span> {m.form.requiredPost}
      </p>
      <div className={cx('field')}>
        <label htmlFor="issue-title">
          {m.form.title}{' '}
          <span className={cx('req')} aria-hidden="true">
            *
          </span>
        </label>
        <input
          type="text"
          id="issue-title"
          name="title"
          value={state.title}
          onChange={(e) => dispatch({ type: 'field', field: 'title', value: e.target.value })}
          required
          data-error="issue-title-error"
          aria-invalid={errors?.title ? 'true' : 'false'}
          aria-describedby={errors?.title ? 'issue-title-api-error' : undefined}
        />
        <span id="issue-title-error" className={cx('error-msg')}>
          {m.form.titleRequired}
        </span>
        {errors?.title && (
          <em className={cx('error-msg')} id="issue-title-api-error" role="alert">
            {errors.title}
          </em>
        )}
      </div>
      <div className={cx('field')}>
        <label htmlFor="issue-body">{m.form.body}</label>
        <textarea
          id="issue-body"
          className={styles.body}
          name="body"
          value={state.body}
          onChange={(e) => dispatch({ type: 'field', field: 'body', value: e.target.value })}
          onKeyDown={submitOnCtrlEnter}
          rows={10}
          aria-invalid={errors?.body ? 'true' : 'false'}
          aria-describedby={
            errors?.body ? 'issue-body-api-error issue-body-hint' : 'issue-body-hint'
          }
        />
        {errors?.body && (
          <em className={cx('error-msg')} id="issue-body-api-error" role="alert">
            {errors.body}
          </em>
        )}
        <p className={cx('field-hint')} id="issue-body-hint">
          {m.form.bodyHint}
        </p>
      </div>
      {labels && labels.length > 0 && (
        <fieldset className={cx('field label-picker')}>
          <legend>{m.form.labels}</legend>
          {labels.map((l) => (
            <label className={cx('label-option')} key={l.id} htmlFor={`label-${l.id}`}>
              <input
                type="checkbox"
                id={`label-${l.id}`}
                name="label_ids"
                value={l.id}
                checked={state.label_ids.includes(l.id)}
                onChange={() => dispatch({ type: 'toggle-label', id: l.id })}
              />
              <LabelBadge label={l} />
            </label>
          ))}
          {errors?.labels && (
            <em className={cx('error-msg')} id="issue-labels-api-error" role="alert">
              {errors.labels}
            </em>
          )}
        </fieldset>
      )}
      <div className={cx('form-actions')}>
        {cancelLink ?? (
          <Link className={cx('btn btn-quiet')} to="/issues">
            {m.copy.cancel}
          </Link>
        )}
        <button type="submit" className={cx('btn btn-primary')} disabled={submitting}>
          {submitting ? m.form.saving : (submitLabel ?? m.issues.create)}
        </button>
      </div>
    </form>
  )
}
