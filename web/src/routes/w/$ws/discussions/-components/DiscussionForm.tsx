import { useReducer, type ReactNode } from 'react'
import type { DiscussionCategory, DiscussionInput, Label } from '../../../../../api'
import { submitOnCtrlEnter } from '../../../../../enter-submit'
import { useMessages } from '../../../../../i18n'
import { useLeaveGuard } from '../../../../../leave-guard'
import { LabelBadge } from '../../../../../components/LabelBadge'
import { cx } from '../../../../../styles/cx'

type DiscussionFormState = {
  title: string
  body: string
  category_id: number | null
  label_ids: number[]
  submitted: boolean
}
type DiscussionFormAction =
  | { type: 'field'; field: 'title' | 'body'; value: string }
  | { type: 'category'; id: number }
  | { type: 'toggle-label'; id: number }
  | { type: 'submitted' }

const discussionFormReducer = (
  s: DiscussionFormState,
  a: DiscussionFormAction,
): DiscussionFormState => {
  switch (a.type) {
    case 'field':
      return { ...s, [a.field]: a.value, submitted: false }
    case 'category':
      return { ...s, category_id: a.id, submitted: false }
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

// New/edit discussion — like IssueForm plus a required category select
// (the MPA's category_id). The qa vs open format only affects the
// comment surface afterwards, not this form.
export function DiscussionForm({
  onSubmit,
  submitting,
  errors,
  initial,
  categories,
  labels,
  submitLabel,
  cancelLink,
}: {
  onSubmit: (input: DiscussionInput) => void
  submitting?: boolean
  errors?: Record<string, string>
  initial?: {
    title: string
    body: string
    category_id?: number | undefined
    label_ids?: number[] | undefined
  }
  categories: DiscussionCategory[]
  labels?: Label[]
  submitLabel?: string
  cancelLink?: ReactNode
}) {
  const init = {
    title: initial?.title ?? '',
    body: initial?.body ?? '',
    category_id: initial?.category_id ?? null,
    label_ids: initial?.label_ids ?? [],
  }
  const [state, dispatch] = useReducer(discussionFormReducer, { ...init, submitted: false })
  const m = useMessages()
  useLeaveGuard(
    !state.submitted &&
      (state.title !== init.title ||
        state.body !== init.body ||
        state.category_id !== init.category_id ||
        state.label_ids.length !== init.label_ids.length ||
        state.label_ids.some((id) => !init.label_ids.includes(id))),
    m.form.unsavedChanges,
  )
  return (
    <form
      className={cx('card')}
      aria-label={initial ? m.discussions.editAria : m.discussions.newAria}
      onSubmit={(e) => {
        e.preventDefault()
        dispatch({ type: 'submitted' })
        onSubmit({
          title: state.title,
          body: state.body,
          category_id: state.category_id ?? undefined,
          label_ids: state.label_ids,
        })
      }}
    >
      <p className={cx('form-note')}>
        {m.form.requiredPre} <span className={cx('req')}>*</span> {m.form.requiredPost}
      </p>
      <div className={cx('field')}>
        <label htmlFor="discussion-category">
          {m.discussions.category}{' '}
          <span className={cx('req')} aria-hidden="true">
            *
          </span>
        </label>
        <select
          id="discussion-category"
          name="category_id"
          value={state.category_id ?? ''}
          onChange={(e) => dispatch({ type: 'category', id: Number(e.target.value) })}
          required
          data-error="discussion-category-error"
          aria-invalid={errors?.category ? 'true' : 'false'}
          aria-describedby={errors?.category ? 'discussion-category-api-error' : undefined}
        >
          <option value="" disabled>
            {m.form.select}
          </option>
          {categories.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
        <span id="discussion-category-error" className={cx('error-msg')}>
          {m.discussions.categoryRequired}
        </span>
        {errors?.category && (
          <em className={cx('error-msg')} id="discussion-category-api-error" role="alert">
            {errors.category}
          </em>
        )}
      </div>
      <div className={cx('field')}>
        <label htmlFor="discussion-title">
          {m.form.title}{' '}
          <span className={cx('req')} aria-hidden="true">
            *
          </span>
        </label>
        <input
          type="text"
          id="discussion-title"
          name="title"
          value={state.title}
          onChange={(e) => dispatch({ type: 'field', field: 'title', value: e.target.value })}
          required
          data-error="discussion-title-error"
          aria-invalid={errors?.title ? 'true' : 'false'}
          aria-describedby={errors?.title ? 'discussion-title-api-error' : undefined}
        />
        <span id="discussion-title-error" className={cx('error-msg')}>
          {m.form.titleRequired}
        </span>
        {errors?.title && (
          <em className={cx('error-msg')} id="discussion-title-api-error" role="alert">
            {errors.title}
          </em>
        )}
      </div>
      <div className={cx('field')}>
        <label htmlFor="discussion-body">{m.form.body}</label>
        <textarea
          id="discussion-body"
          name="body"
          value={state.body}
          onChange={(e) => dispatch({ type: 'field', field: 'body', value: e.target.value })}
          onKeyDown={submitOnCtrlEnter}
          rows={10}
          aria-invalid={errors?.body ? 'true' : 'false'}
          aria-describedby={
            errors?.body ? 'discussion-body-api-error discussion-body-hint' : 'discussion-body-hint'
          }
        />
        {errors?.body && (
          <em className={cx('error-msg')} id="discussion-body-api-error" role="alert">
            {errors.body}
          </em>
        )}
        <p className={cx('field-hint')} id="discussion-body-hint">
          {m.form.bodyHint}
        </p>
      </div>
      {labels && labels.length > 0 && (
        <fieldset className={cx('field label-picker')}>
          <legend>{m.form.labels}</legend>
          {labels.map((l) => (
            <label className={cx('label-option')} key={l.id} htmlFor={`dlabel-${l.id}`}>
              <input
                type="checkbox"
                id={`dlabel-${l.id}`}
                name="label_ids"
                value={l.id}
                checked={state.label_ids.includes(l.id)}
                onChange={() => dispatch({ type: 'toggle-label', id: l.id })}
              />
              <LabelBadge label={l} />
            </label>
          ))}
          {errors?.labels && (
            <em className={cx('error-msg')} id="discussion-labels-api-error" role="alert">
              {errors.labels}
            </em>
          )}
        </fieldset>
      )}
      <div className={cx('form-actions')}>
        {cancelLink}
        <button type="submit" className={cx('btn btn-primary')} disabled={submitting}>
          {submitting ? m.form.saving : (submitLabel ?? m.discussions.create)}
        </button>
      </div>
    </form>
  )
}
