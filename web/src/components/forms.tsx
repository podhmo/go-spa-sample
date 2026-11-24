import { useReducer } from 'react'
import type { IssueInput } from '../api'

type IssueFormState = { title: string; body: string }
type IssueFormAction = { field: keyof IssueFormState; value: string }

export function IssueForm({
  onSubmit,
  submitting,
  errors,
}: {
  onSubmit: (input: IssueInput) => void
  submitting?: boolean
  errors?: Record<string, string>
}) {
  const [state, dispatch] = useReducer(
    (s: IssueFormState, a: IssueFormAction) => ({ ...s, [a.field]: a.value }),
    { title: '', body: '' },
  )
  return (
    <form
      aria-label="new issue"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(state)
      }}
    >
      <div className="field">
        <label htmlFor="issue-title">
          Title <span aria-hidden="true">*</span>
        </label>
        <input
          id="issue-title"
          name="title"
          value={state.title}
          onChange={(e) => dispatch({ field: 'title', value: e.target.value })}
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
          onChange={(e) => dispatch({ field: 'body', value: e.target.value })}
          rows={6}
          aria-invalid={errors?.body ? 'true' : 'false'}
          aria-describedby={errors?.body ? 'issue-body-api-error' : undefined}
        />
        {errors?.body && (
          <em id="issue-body-api-error" role="alert">
            {errors.body}
          </em>
        )}
      </div>
      <button type="submit" disabled={submitting}>
        {submitting ? 'creating…' : 'create issue'}
      </button>
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
      aria-label="new comment"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(body)
        setBody('')
      }}
    >
      <div className="field">
        <label htmlFor="comment-body">
          Comment <span aria-hidden="true">*</span>
        </label>
        <textarea
          id="comment-body"
          name="body"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={4}
          required
          data-error="comment-body-error"
          aria-invalid="false"
        />
        <span id="comment-body-error" className="error-msg">
          Comment is required.
        </span>
      </div>
      <button type="submit" disabled={submitting}>
        {submitting ? 'posting…' : 'post comment'}
      </button>
    </form>
  )
}
