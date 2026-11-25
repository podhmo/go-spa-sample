import { useReducer } from 'react'
import type { Label, LabelInput } from '../../../../../api'
import { ConfirmDialog } from '../../../../../components/ConfirmDialog'
import { fmt, useMessages, type Messages } from '../../../../../i18n'
import { LabelBadge } from '../../../../../components/LabelBadge'
import styles from './LabelsAdmin.module.css'
import { cx } from '../../../../../styles/cx'

const scopeName = (m: Messages, l: Label) =>
  l.workspace_id == null ? m.labels.scopeNameShared : m.labels.scopeNameWorkspace
const kindName = (m: Messages, kind: string) =>
  kind === 'discussion' ? m.labels.kindDiscussion : m.labels.kindIssue

type LabelFormState = { name: string; color: string }
type LabelFormAction = { type: 'field'; field: keyof LabelFormState; value: string }

// Inline per-row editor: name + color are the only mutable fields
// (kind and workspace scope are fixed at creation).
function LabelRow({
  label: l,
  archived,
  pending,
  onUpdate,
  onArchive,
  onRestore,
}: {
  label: Label
  archived?: boolean | undefined
  pending?: boolean | undefined
  onUpdate: (id: number, patch: { name?: string; color?: string }) => void
  onArchive?: ((id: number) => void) | undefined
  onRestore?: ((label: Label) => void) | undefined
}) {
  const [ui, dispatch] = useReducer(
    (
      s: { editing: boolean } & LabelFormState,
      a: { type: 'edit'; name: string; color: string } | { type: 'cancel' } | LabelFormAction,
    ) => {
      if (a.type === 'edit') return { editing: true, name: a.name, color: a.color }
      if (a.type === 'cancel') return { ...s, editing: false }
      return { ...s, [a.field]: a.value }
    },
    { editing: false, name: l.name, color: l.color },
  )
  const m = useMessages()
  if (ui.editing) {
    return (
      <li>
        <form
          className={cx('validation-scope')}
          aria-label={fmt(m.labels.editAria, { name: l.name })}
          onSubmit={(e) => {
            e.preventDefault()
            onUpdate(l.id, { name: ui.name, color: ui.color })
            dispatch({ type: 'cancel' })
          }}
        >
          <label htmlFor={`label-name-${l.id}`} className="visually-hidden">
            {m.labels.name}
          </label>
          <input
            type="text"
            id={`label-name-${l.id}`}
            value={ui.name}
            onChange={(e) => dispatch({ type: 'field', field: 'name', value: e.target.value })}
            required
          />
          <label htmlFor={`label-color-${l.id}`} className="visually-hidden">
            {m.labels.color}
          </label>
          <input
            id={`label-color-${l.id}`}
            type="color"
            value={ui.color}
            onChange={(e) => dispatch({ type: 'field', field: 'color', value: e.target.value })}
            required
          />
          <div className={cx('form-actions')}>
            <button
              type="button"
              className={cx('btn btn-quiet')}
              onClick={() => dispatch({ type: 'cancel' })}
            >
              {m.copy.cancel}
            </button>
            <button type="submit" className={cx('btn btn-primary')} disabled={pending}>
              {m.form.saveChanges}
            </button>
          </div>
        </form>
      </li>
    )
  }
  // Plain <li> like the MPA: badge, scope (and kind for archived rows),
  // then quiet action buttons flowing inline.
  return (
    <li>
      <LabelBadge label={l} />{' '}
      <span className={styles.labelScope}>
        {scopeName(m, l)}
        {archived ? ` · ${kindName(m, l.kind)}` : ''}
      </span>{' '}
      <button
        type="button"
        className={cx('btn btn-quiet')}
        onClick={() => dispatch({ type: 'edit', name: l.name, color: l.color })}
      >
        {m.labels.edit}
      </button>{' '}
      {archived
        ? onRestore && (
            <button
              type="button"
              className={cx('btn btn-quiet')}
              disabled={pending}
              onClick={() => onRestore(l)}
            >
              {m.labels.restore}
            </button>
          )
        : onArchive && (
            <ConfirmDialog
              label={m.labels.delete}
              title={fmt(m.labels.deleteTitle, { name: l.name })}
              description={m.labels.deleteBody}
              confirmLabel={m.labels.delete}
              triggerClass={cx('btn btn-quiet')}
              pending={pending}
              onConfirm={() => onArchive(l.id)}
            />
          )}
    </li>
  )
}

function LabelGroup({
  id,
  title,
  empty,
  labels,
  pending,
  onUpdate,
  onArchive,
}: {
  id: string
  title: string
  empty: string
  labels: Label[]
  pending?: boolean | undefined
  onUpdate: (id: number, patch: { name?: string; color?: string }) => void
  onArchive?: ((id: number) => void) | undefined
}) {
  return (
    <section aria-labelledby={id}>
      <h2 id={id}>{title}</h2>
      {labels.length === 0 ? (
        <p className="empty-state">{empty}</p>
      ) : (
        <ul className={styles.labelList} role="list">
          {labels.map((l) => (
            <LabelRow
              key={l.id}
              label={l}
              pending={pending}
              onUpdate={onUpdate}
              onArchive={onArchive}
            />
          ))}
        </ul>
      )}
    </section>
  )
}

export function LabelsAdmin({
  active,
  archived,
  pending,
  errors,
  onCreate,
  onUpdate,
  onArchive,
  onRestore,
}: {
  active: Label[]
  archived: Label[]
  pending?: boolean
  errors?: Record<string, string>
  onCreate: (input: LabelInput) => void
  onUpdate: (id: number, patch: { name?: string; color?: string }) => void
  onArchive: (id: number) => void
  // Restoring is a re-create of the same name/kind/scope: the store
  // revives the archived row and applies the new color.
  onRestore: (label: Label) => void
}) {
  const [form, dispatch] = useReducer(
    (
      s: LabelFormState & { kind: string; shared: boolean },
      a: LabelFormAction | { type: 'kind'; value: string } | { type: 'scope'; value: string },
    ) => {
      if (a.type === 'kind') return { ...s, kind: a.value }
      if (a.type === 'scope') return { ...s, shared: a.value === 'shared' }
      return { ...s, [a.field]: a.value }
    },
    { name: '', color: '#888888', kind: 'issue', shared: false },
  )
  const m = useMessages()
  return (
    <div>
      <div className={cx('page-head')}>
        <h1>{m.labels.title}</h1>
      </div>
      <section
        className={`${cx('card')} ${styles.labelAdminForm}`}
        aria-labelledby="new-label-heading"
      >
        <h2 id="new-label-heading">{m.labels.new}</h2>
        <form
          className="form-inline"
          aria-label={m.labels.newAria}
          onSubmit={(e) => {
            e.preventDefault()
            onCreate({ name: form.name, color: form.color, kind: form.kind, shared: form.shared })
          }}
        >
          <div className={cx('field')}>
            <label htmlFor="new-label-name">{m.labels.name}</label>
            <input
              type="text"
              id="new-label-name"
              name="name"
              value={form.name}
              onChange={(e) => dispatch({ type: 'field', field: 'name', value: e.target.value })}
              required
              maxLength={64}
              pattern=".*\S.*"
              autoComplete="off"
              data-error="new-label-name-error"
              aria-invalid={errors?.name ? 'true' : 'false'}
              aria-describedby={errors?.name ? 'new-label-name-api-error' : undefined}
            />
            <span id="new-label-name-error" className={cx('error-msg')}>
              {m.labels.nameRequired}
            </span>
            {errors?.name && (
              <em className={cx('error-msg')} id="new-label-name-api-error" role="alert">
                {errors.name}
              </em>
            )}
          </div>
          <div className={cx('field')}>
            <label htmlFor="new-label-color">{m.labels.color}</label>
            <input
              id="new-label-color"
              type="color"
              name="color"
              value={form.color}
              onChange={(e) => dispatch({ type: 'field', field: 'color', value: e.target.value })}
              required
              aria-invalid={errors?.color ? 'true' : 'false'}
              aria-describedby={errors?.color ? 'new-label-color-api-error' : undefined}
            />
            {errors?.color && (
              <em className={cx('error-msg')} id="new-label-color-api-error" role="alert">
                {errors.color}
              </em>
            )}
          </div>
          <div className={cx('field')}>
            <label htmlFor="new-label-kind">{m.labels.kind}</label>
            <select
              id="new-label-kind"
              name="kind"
              value={form.kind}
              onChange={(e) => dispatch({ type: 'kind', value: e.target.value })}
            >
              <option value="issue">{m.labels.kindIssue}</option>
              <option value="discussion">{m.labels.kindDiscussion}</option>
            </select>
          </div>
          <div className={cx('field')}>
            <label htmlFor="new-label-scope">{m.labels.scope}</label>
            <select
              id="new-label-scope"
              name="scope"
              value={form.shared ? 'shared' : 'workspace'}
              onChange={(e) => dispatch({ type: 'scope', value: e.target.value })}
            >
              <option value="workspace">{m.labels.scopeWorkspace}</option>
              <option value="shared">{m.labels.scopeShared}</option>
            </select>
          </div>
          {/* the MPA's form-inline keeps the submit unwrapped at the
              bottom-left of the form */}
          <button type="submit" className={cx('btn btn-primary')} disabled={pending}>
            {m.labels.create}
          </button>
        </form>
      </section>
      <LabelGroup
        id="issue-labels-heading"
        title={m.labels.issueLabels}
        empty={m.labels.noIssue}
        labels={active.filter((l) => l.kind === 'issue')}
        pending={pending}
        onUpdate={onUpdate}
        onArchive={onArchive}
      />
      <LabelGroup
        id="discussion-labels-heading"
        title={m.labels.discussionLabels}
        empty={m.labels.noDiscussion}
        labels={active.filter((l) => l.kind === 'discussion')}
        pending={pending}
        onUpdate={onUpdate}
        onArchive={onArchive}
      />
      {/* the MPA shows the Archived section only when it has rows */}
      {archived.length > 0 && (
        <section aria-labelledby="archived-labels-heading">
          <h2 id="archived-labels-heading">{m.labels.archived}</h2>
          <p className={cx('field-hint')}>{m.labels.archivedHint}</p>
          <ul className={styles.labelList} role="list">
            {archived.map((l) => (
              <LabelRow
                key={l.id}
                label={l}
                archived
                pending={pending}
                onUpdate={onUpdate}
                onRestore={onRestore}
              />
            ))}
          </ul>
        </section>
      )}
    </div>
  )
}
