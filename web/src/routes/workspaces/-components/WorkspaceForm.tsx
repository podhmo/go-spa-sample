import { useReducer, type ReactNode } from 'react'
import type { WorkspaceInput } from '../../../api'
import { useMessages } from '../../../i18n'
import { useLeaveGuard } from '../../../leave-guard'
import { cx } from '../../../styles/cx'

type WorkspaceFormState = { slug: string; name: string; description: string }

const workspaceFormReducer = (
  s: WorkspaceFormState & { submitted: boolean },
  a: { field?: keyof WorkspaceFormState; value?: string; submitted?: boolean },
): WorkspaceFormState & { submitted: boolean } =>
  a.submitted
    ? { ...s, submitted: true }
    : { ...s, [a.field as keyof WorkspaceFormState]: a.value, submitted: false }

// The MPA's /workspaces/new form — slug (immutable identifier, used in
// /w/<slug>/... URLs), optional display name and description.
export function WorkspaceForm({
  onSubmit,
  submitting,
  errors,
  cancelLink,
}: {
  onSubmit: (input: WorkspaceInput) => void
  submitting?: boolean
  errors?: Record<string, string>
  cancelLink?: ReactNode
}) {
  const [state, dispatch] = useReducer(workspaceFormReducer, {
    slug: '',
    name: '',
    description: '',
    submitted: false,
  })
  const m = useMessages()
  useLeaveGuard(
    !state.submitted && Boolean(state.slug || state.name || state.description),
    m.form.unsavedChanges,
  )
  return (
    <form
      className={cx('card issue-form')}
      aria-label={m.workspaces.formAria}
      onSubmit={(e) => {
        e.preventDefault()
        dispatch({ submitted: true })
        const { submitted: _submitted, ...input } = state
        onSubmit(input)
      }}
    >
      <p className={cx('form-note')}>
        {m.form.requiredPre} <span className={cx('req')}>*</span> {m.form.requiredPost}
      </p>
      <div className={cx('field')}>
        <label htmlFor="workspace-slug">
          {m.workspaces.slug}{' '}
          <span className={cx('req')} aria-hidden="true">
            *
          </span>
        </label>
        <input
          type="text"
          id="workspace-slug"
          name="slug"
          value={state.slug}
          onChange={(e) => dispatch({ field: 'slug', value: e.target.value })}
          required
          pattern="[a-z0-9][a-z0-9_\-]*"
          maxLength={63}
          autoComplete="off"
          data-error="workspace-slug-error"
          aria-invalid={errors?.slug ? 'true' : 'false'}
          aria-describedby={
            errors?.slug ? 'workspace-slug-api-error workspace-slug-hint' : 'workspace-slug-hint'
          }
        />
        <span id="workspace-slug-error" className={cx('error-msg')}>
          {m.workspaces.slugPattern}
        </span>
        {errors?.slug && (
          <em className={cx('error-msg')} id="workspace-slug-api-error" role="alert">
            {errors.slug}
          </em>
        )}
        <p className={cx('field-hint')} id="workspace-slug-hint">
          {m.workspaces.slugHint}
        </p>
      </div>
      <div className={cx('field')}>
        <label htmlFor="workspace-name">{m.workspaces.name}</label>
        <input
          type="text"
          id="workspace-name"
          name="name"
          value={state.name}
          onChange={(e) => dispatch({ field: 'name', value: e.target.value })}
          maxLength={200}
          autoComplete="off"
          aria-invalid={errors?.name ? 'true' : 'false'}
          aria-describedby="workspace-name-hint"
        />
        {errors?.name && (
          <em className={cx('error-msg')} id="workspace-name-api-error" role="alert">
            {errors.name}
          </em>
        )}
        <p className={cx('field-hint')} id="workspace-name-hint">
          {m.workspaces.nameHint}
        </p>
      </div>
      <div className={cx('field')}>
        <label htmlFor="workspace-description">{m.workspaces.description}</label>
        <textarea
          id="workspace-description"
          name="description"
          value={state.description}
          onChange={(e) => dispatch({ field: 'description', value: e.target.value })}
          rows={3}
          aria-invalid={errors?.description ? 'true' : 'false'}
        />
        {errors?.description && (
          <em className={cx('error-msg')} id="workspace-description-api-error" role="alert">
            {errors.description}
          </em>
        )}
      </div>
      <div className={cx('form-actions')}>
        {cancelLink}
        <button type="submit" className={cx('btn btn-primary')} disabled={submitting}>
          {submitting ? m.form.saving : m.workspaces.create}
        </button>
      </div>
    </form>
  )
}
