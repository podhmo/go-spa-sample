import { Link } from '@tanstack/react-router'
import { useReducer, type ReactNode } from 'react'
import type {
  DiscussionCategory,
  DiscussionInput,
  IssueInput,
  Label,
  PullRef,
  WorkspaceInput,
} from '../api'
import { submitOnCtrlEnter } from '../enter-submit'
import { useMessages } from '../i18n'
import { useLeaveGuard } from '../leave-guard'
import { LabelBadge } from './IssueList'

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
      className="card issue-form"
      aria-label={initial ? m.issues.editAria : m.issues.newAria}
      onSubmit={(e) => {
        e.preventDefault()
        dispatch({ type: 'submitted' })
        const { submitted: _submitted, ...input } = state
        onSubmit(input)
      }}
    >
      <p className="form-note">
        {m.form.requiredPre} <span className="req">*</span> {m.form.requiredPost}
      </p>
      <div className="field">
        <label htmlFor="issue-title">
          {m.form.title}{' '}
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
          {m.form.titleRequired}
        </span>
        {errors?.title && (
          <em id="issue-title-api-error" role="alert">
            {errors.title}
          </em>
        )}
      </div>
      <div className="field">
        <label htmlFor="issue-body">{m.form.body}</label>
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
          {m.form.bodyHint}
        </p>
      </div>
      {labels && labels.length > 0 && (
        <fieldset className="field label-picker">
          <legend>{m.form.labels}</legend>
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
            {m.copy.cancel}
          </Link>
        )}
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? m.form.saving : (submitLabel ?? m.issues.create)}
        </button>
      </div>
    </form>
  )
}

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
      className="card issue-form"
      aria-label={m.workspaces.formAria}
      onSubmit={(e) => {
        e.preventDefault()
        dispatch({ submitted: true })
        const { submitted: _submitted, ...input } = state
        onSubmit(input)
      }}
    >
      <p className="form-note">
        {m.form.requiredPre} <span className="req">*</span> {m.form.requiredPost}
      </p>
      <div className="field">
        <label htmlFor="workspace-slug">
          {m.workspaces.slug}{' '}
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
          {m.workspaces.slugPattern}
        </span>
        {errors?.slug && (
          <em id="workspace-slug-api-error" role="alert">
            {errors.slug}
          </em>
        )}
        <p className="field-hint" id="workspace-slug-hint">
          {m.workspaces.slugHint}
        </p>
      </div>
      <div className="field">
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
          <em id="workspace-name-api-error" role="alert">
            {errors.name}
          </em>
        )}
        <p className="field-hint" id="workspace-name-hint">
          {m.workspaces.nameHint}
        </p>
      </div>
      <div className="field">
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
          <em id="workspace-description-api-error" role="alert">
            {errors.description}
          </em>
        )}
      </div>
      <div className="form-actions">
        {cancelLink}
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? m.form.saving : m.workspaces.create}
        </button>
      </div>
    </form>
  )
}

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
      className="card issue-form"
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
      <p className="form-note">
        {m.form.requiredPre} <span className="req">*</span> {m.form.requiredPost}
      </p>
      <div className="field">
        <label htmlFor="discussion-category">
          {m.discussions.category}{' '}
          <span className="req" aria-hidden="true">
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
        <span id="discussion-category-error" className="error-msg">
          {m.discussions.categoryRequired}
        </span>
        {errors?.category && (
          <em id="discussion-category-api-error" role="alert">
            {errors.category}
          </em>
        )}
      </div>
      <div className="field">
        <label htmlFor="discussion-title">
          {m.form.title}{' '}
          <span className="req" aria-hidden="true">
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
        <span id="discussion-title-error" className="error-msg">
          {m.form.titleRequired}
        </span>
        {errors?.title && (
          <em id="discussion-title-api-error" role="alert">
            {errors.title}
          </em>
        )}
      </div>
      <div className="field">
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
          <em id="discussion-body-api-error" role="alert">
            {errors.body}
          </em>
        )}
        <p className="field-hint" id="discussion-body-hint">
          {m.form.bodyHint}
        </p>
      </div>
      {labels && labels.length > 0 && (
        <fieldset className="field label-picker">
          <legend>{m.form.labels}</legend>
          {labels.map((l) => (
            <label className="label-option" key={l.id} htmlFor={`dlabel-${l.id}`}>
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
            <em id="discussion-labels-api-error" role="alert">
              {errors.labels}
            </em>
          )}
        </fieldset>
      )}
      <div className="form-actions">
        {cancelLink}
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? m.form.saving : (submitLabel ?? m.discussions.create)}
        </button>
      </div>
    </form>
  )
}

export function CommentForm({
  onSubmit,
  submitting,
}: {
  onSubmit: (body: string) => void | boolean | Promise<void | boolean>
  submitting?: boolean
}) {
  const [body, setBody] = useReducer((_: string, v: string) => v, '')
  const m = useMessages()
  useLeaveGuard(body.trim() !== '', m.form.unsavedChanges)
  return (
    <form
      className="card comment-form"
      aria-label={m.form.newCommentAria}
      onSubmit={(e) => {
        e.preventDefault()
        const form = e.currentTarget
        void (async () => {
          const ok = await onSubmit(body)
          if (ok !== false) {
            // Native reset clears the :user-invalid interaction state;
            // clearing only the controlled value leaves it looking invalid.
            form.reset()
            setBody('')
          }
        })()
      }}
    >
      <div className="field">
        <label htmlFor="comment-body">
          {m.form.addComment}{' '}
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
          {m.form.bodyRequired}
        </span>
        <p className="field-hint" id="comment-body-hint">
          {m.form.bodyHintShort}
        </p>
      </div>
      <div className="form-actions">
        {/* disabled while empty: an empty post can only 422, so the
            GitHub-style rule disables the button until there is input
            (ui-rules §6) */}
        <button type="submit" className="btn btn-primary" disabled={submitting || !body.trim()}>
          {submitting ? m.form.posting : m.pulls.commentSubmit}
        </button>
      </div>
    </form>
  )
}

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
      className="card issue-form"
      aria-label={editing ? m.pulls.editAria : m.pulls.newAria}
      onSubmit={(e) => {
        e.preventDefault()
        dispatch({ type: 'submitted' })
        const { submitted: _submitted, ...input } = state
        onSubmit(editing ? { title: input.title, body: input.body } : input)
      }}
    >
      <p className="form-note">
        {m.form.requiredPre}{' '}
        <span className="req" aria-hidden="true">
          *
        </span>{' '}
        {m.form.requiredPost}
      </p>
      {!editing && (
        <div className="pull-refs-row">
          <div className="field">
            <label htmlFor="pull-head">
              {m.pulls.head}{' '}
              <span className="req" aria-hidden="true">
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
            <span id="pull-head-error" className="error-msg">
              {m.pulls.headRequired}
            </span>
            {errors?.head && (
              <em id="pull-head-api-error" role="alert">
                {errors.head}
              </em>
            )}
          </div>
          <div className="field">
            <label htmlFor="pull-base">
              {m.pulls.base}{' '}
              <span className="req" aria-hidden="true">
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
            <span id="pull-base-error" className="error-msg">
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
      <div className="field">
        <label htmlFor="pull-title">
          {m.form.title}{' '}
          <span className="req" aria-hidden="true">
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
        <span id="pull-title-error" className="error-msg">
          {m.form.titleRequired}
        </span>
        {errors?.title && (
          <em id="pull-title-api-error" role="alert">
            {errors.title}
          </em>
        )}
      </div>
      <div className="field">
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
      <div className="form-actions">
        {cancelLink}
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting
            ? m.form.saving
            : (submitLabel ?? (editing ? m.form.saveChanges : m.pulls.create))}
        </button>
      </div>
    </form>
  )
}

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
      className="card issue-form"
      aria-label={mode === 'login' ? m.auth.logInAria : m.auth.registerAria}
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(state.name, state.password)
      }}
    >
      <p className="form-note">
        {m.form.requiredPre}{' '}
        <span className="req" aria-hidden="true">
          *
        </span>{' '}
        {m.form.requiredPost}
      </p>
      {errors?.form && (
        <em className="form-error" role="alert">
          {errors.form}
        </em>
      )}
      <div className="field">
        <label htmlFor={`${id}-name`}>
          {m.form.name}{' '}
          <span className="req" aria-hidden="true">
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
        <span id={`${id}-name-error`} className="error-msg">
          {m.form.nameRequired}
        </span>
        {errors?.name && (
          <em id={`${id}-name-api-error`} role="alert">
            {errors.name}
          </em>
        )}
      </div>
      <div className="field">
        <label htmlFor={`${id}-password`}>
          {m.form.password}{' '}
          <span className="req" aria-hidden="true">
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
        <span id={`${id}-password-error`} className="error-msg">
          {mode === 'register' ? m.form.passwordMin : m.form.passwordRequired}
        </span>
        {errors?.password && (
          <em id={`${id}-password-api-error`} role="alert">
            {errors.password}
          </em>
        )}
      </div>
      <div className="form-actions">
        <button type="submit" className="btn btn-primary" disabled={submitting}>
          {submitting ? '…' : mode === 'login' ? m.nav.logIn : m.nav.register}
        </button>
      </div>
    </form>
  )
}
