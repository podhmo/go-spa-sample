import { Link } from '@tanstack/react-router'
import { useReducer, type ReactNode } from 'react'
import type { IssueInput, Label, WorkspaceInput } from '../api'
import { submitOnCtrlEnter } from '../enter-submit'
import { LabelBadge } from './IssueList'

type IssueFormState = { title: string; body: string; label_ids: number[] }
type IssueFormAction =
  | { type: 'field'; field: 'title' | 'body'; value: string }
  | { type: 'toggle-label'; id: number }

const issueFormReducer = (s: IssueFormState, a: IssueFormAction): IssueFormState => {
  switch (a.type) {
    case 'field':
      return { ...s, [a.field]: a.value }
    case 'toggle-label':
      return {
        ...s,
        label_ids: s.label_ids.includes(a.id)
          ? s.label_ids.filter((id) => id !== a.id)
          : [...s.label_ids, a.id],
      }
  }
}

export function IssueForm({
  onSubmit,
  submitting,
  errors,
  initial,
  labels,
  submitLabel = 'Create issue',
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
  const [state, dispatch] = useReducer(issueFormReducer, {
    title: initial?.title ?? '',
    body: initial?.body ?? '',
    label_ids: initial?.label_ids ?? [],
  })
  return (
    <form
      className="card issue-form"
      aria-label={initial ? 'edit issue' : 'new issue'}
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(state)
      }}
    >
      <p className="form-note">
        Fields marked <span className="req">*</span> are required.
      </p>
      <div className="field">
        <label htmlFor="issue-title">
          Title{' '}
          <span className="req" aria-hidden="true">
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
        <span id="issue-title-error" className="error-msg">
          Title is required.
        </span>
        {errors?.title && (
          <em id="issue-title-api-error" role="alert">
            {errors.title}
          </em>
        )}
      </div>
      <div className="field">
        <label htmlFor="issue-body">Body</label>
        <textarea
          id="issue-body"
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
          <em id="issue-body-api-error" role="alert">
            {errors.body}
          </em>
        )}
        <p className="field-hint" id="issue-body-hint">
          Markdown (GFM) supported. Raw HTML is not rendered; line breaks are preserved. Ctrl+Enter
          (Cmd+Enter on macOS) submits.
        </p>
      </div>
      {labels && labels.length > 0 && (
        <fieldset className="field label-picker">
          <legend>Labels</legend>
          {labels.map((l) => (
            <label className="label-option" key={l.id} htmlFor={`label-${l.id}`}>
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
            <em id="issue-labels-api-error" role="alert">
              {errors.labels}
            </em>
          )}
        </fieldset>
      )}
      <div className="form-actions">
        {cancelLink ?? (
          <Link className="btn btn-quiet" to="/issues">
            Cancel
          </Link>
        )}
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? 'Saving…' : submitLabel}
        </button>
      </div>
    </form>
  )
}

type WorkspaceFormState = { slug: string; name: string; description: string }

const workspaceFormReducer = (
  s: WorkspaceFormState,
  a: { field: keyof WorkspaceFormState; value: string },
): WorkspaceFormState => ({ ...s, [a.field]: a.value })

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
  })
  return (
    <form
      className="card issue-form"
      aria-label="new workspace"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(state)
      }}
    >
      <p className="form-note">
        Fields marked <span className="req">*</span> are required.
      </p>
      <div className="field">
        <label htmlFor="workspace-slug">
          Slug{' '}
          <span className="req" aria-hidden="true">
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
        <span id="workspace-slug-error" className="error-msg">
          Lowercase letters, digits, &quot;-&quot; and &quot;_&quot;, starting with a letter or
          digit.
        </span>
        {errors?.slug && (
          <em id="workspace-slug-api-error" role="alert">
            {errors.slug}
          </em>
        )}
        <p className="field-hint" id="workspace-slug-hint">
          Lowercase letters, digits, &quot;-&quot; and &quot;_&quot;. Used in URLs:
          /w/&lt;slug&gt;/issues.
        </p>
      </div>
      <div className="field">
        <label htmlFor="workspace-name">Name</label>
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
          <em id="workspace-name-api-error" role="alert">
            {errors.name}
          </em>
        )}
        <p className="field-hint" id="workspace-name-hint">
          Optional display name; defaults to the slug.
        </p>
      </div>
      <div className="field">
        <label htmlFor="workspace-description">Description</label>
        <textarea
          id="workspace-description"
          name="description"
          value={state.description}
          onChange={(e) => dispatch({ field: 'description', value: e.target.value })}
          rows={3}
          aria-invalid={errors?.description ? 'true' : 'false'}
        />
        {errors?.description && (
          <em id="workspace-description-api-error" role="alert">
            {errors.description}
          </em>
        )}
      </div>
      <div className="form-actions">
        {cancelLink}
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? 'Saving…' : 'Create workspace'}
        </button>
      </div>
    </form>
  )
}

export function CommentForm({
  onSubmit,
  submitting,
}: {
  onSubmit: (body: string) => void
  submitting?: boolean
}) {
  const [body, setBody] = useReducer((_: string, v: string) => v, '')
  return (
    <form
      className="card comment-form"
      aria-label="new comment"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(body)
        setBody('')
      }}
    >
      <div className="field">
        <label htmlFor="comment-body">
          Add a comment{' '}
          <span className="req" aria-hidden="true">
            *
          </span>
        </label>
        <textarea
          id="comment-body"
          name="body"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={submitOnCtrlEnter}
          rows={4}
          required
          data-error="comment-body-error"
          aria-invalid="false"
          aria-describedby="comment-body-hint"
        />
        <span id="comment-body-error" className="error-msg">
          Body is required.
        </span>
        <p className="field-hint" id="comment-body-hint">
          Markdown (GFM) supported. Raw HTML is not rendered. Ctrl+Enter (Cmd+Enter on macOS)
          submits.
        </p>
      </div>
      <div className="form-actions">
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? 'Posting…' : 'Comment'}
        </button>
      </div>
    </form>
  )
}
