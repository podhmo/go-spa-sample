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
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(state)
      }}
    >
      <div>
        <label htmlFor="issue-title">Title</label>{' '}
        <input
          id="issue-title"
          name="title"
          value={state.title}
          onChange={(e) => dispatch({ field: 'title', value: e.target.value })}
          required
        />
        {errors?.title && <em role="alert">{errors.title}</em>}
      </div>
      <div>
        <label htmlFor="issue-body">Body</label>{' '}
        <textarea
          id="issue-body"
          name="body"
          value={state.body}
          onChange={(e) => dispatch({ field: 'body', value: e.target.value })}
          rows={6}
        />
        {errors?.body && <em role="alert">{errors.body}</em>}
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
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(body)
        setBody('')
      }}
    >
      <div>
        <label htmlFor="comment-body">Comment</label>{' '}
        <textarea
          id="comment-body"
          name="body"
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={4}
          required
        />
      </div>
      <button type="submit" disabled={submitting}>
        {submitting ? 'posting…' : 'post comment'}
      </button>
    </form>
  )
}
