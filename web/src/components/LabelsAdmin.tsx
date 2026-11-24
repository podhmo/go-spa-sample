import { useReducer } from 'react'
import type { Label, LabelInput } from '../api'
import { ConfirmDialog } from './ConfirmDialog'
import { LabelBadge } from './IssueList'

const scopeName = (l: Label) => (l.workspace_id == null ? 'shared' : 'workspace')

type LabelFormState = { name: string; color: string }
type LabelFormAction = { type: 'field'; field: keyof LabelFormState; value: string }

// Inline per-row editor: name + color are the only mutable fields
// (kind and workspace scope are fixed at creation).
function LabelRow({
  label: l,
  pending,
  onUpdate,
  onArchive,
}: {
  label: Label
  pending?: boolean
  onUpdate: (id: number, patch: { name?: string; color?: string }) => void
  onArchive?: (id: number) => void
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
  if (ui.editing) {
    return (
      <li>
        <form
          aria-label={`edit label ${l.name}`}
          onSubmit={(e) => {
            e.preventDefault()
            onUpdate(l.id, { name: ui.name, color: ui.color })
            dispatch({ type: 'cancel' })
          }}
        >
          <label htmlFor={`label-name-${l.id}`} className="visually-hidden">
            Name
          </label>
          <input
            id={`label-name-${l.id}`}
            value={ui.name}
            onChange={(e) => dispatch({ type: 'field', field: 'name', value: e.target.value })}
            required
          />{' '}
          <label htmlFor={`label-color-${l.id}`} className="visually-hidden">
            Color
          </label>
          <input
            id={`label-color-${l.id}`}
            type="color"
            value={ui.color}
            onChange={(e) => dispatch({ type: 'field', field: 'color', value: e.target.value })}
            required
          />{' '}
          <button type="submit" disabled={pending}>
            save
          </button>{' '}
          <button type="button" onClick={() => dispatch({ type: 'cancel' })}>
            cancel
          </button>
        </form>
      </li>
    )
  }
  return (
    <li>
      <LabelBadge label={l} /> <small>{scopeName(l)}</small>{' '}
      <button
        type="button"
        onClick={() => dispatch({ type: 'edit', name: l.name, color: l.color })}
      >
        edit
      </button>{' '}
      {onArchive && (
        <ConfirmDialog
          label="archive"
          title={`Archive label "${l.name}"?`}
          confirmLabel="archive"
          pending={pending}
          onConfirm={() => onArchive(l.id)}
        />
      )}
    </li>
  )
}

function LabelGroup({
  title,
  labels,
  pending,
  onUpdate,
  onArchive,
}: {
  title: string
  labels: Label[]
  pending?: boolean
  onUpdate: (id: number, patch: { name?: string; color?: string }) => void
  onArchive?: (id: number) => void
}) {
  return (
    <section>
      <h3>{title}</h3>
      {labels.length === 0 ? (
        <p>none</p>
      ) : (
        <ul>
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
  return (
    <div>
      <h2>Labels</h2>
      <form
        aria-label="new label"
        onSubmit={(e) => {
          e.preventDefault()
          onCreate({ name: form.name, color: form.color, kind: form.kind, shared: form.shared })
        }}
      >
        <label htmlFor="new-label-name" className="visually-hidden">
          Name
        </label>
        <input
          id="new-label-name"
          name="name"
          placeholder="name"
          value={form.name}
          onChange={(e) => dispatch({ type: 'field', field: 'name', value: e.target.value })}
          required
          data-error="new-label-name-error"
          aria-invalid={errors?.name ? 'true' : 'false'}
          aria-describedby={errors?.name ? 'new-label-name-api-error' : undefined}
        />{' '}
        <label htmlFor="new-label-color" className="visually-hidden">
          Color
        </label>
        <input
          id="new-label-color"
          type="color"
          name="color"
          value={form.color}
          onChange={(e) => dispatch({ type: 'field', field: 'color', value: e.target.value })}
          required
          aria-invalid={errors?.color ? 'true' : 'false'}
          aria-describedby={errors?.color ? 'new-label-color-api-error' : undefined}
        />{' '}
        <label htmlFor="new-label-kind" className="visually-hidden">
          Kind
        </label>
        <select
          id="new-label-kind"
          name="kind"
          value={form.kind}
          onChange={(e) => dispatch({ type: 'kind', value: e.target.value })}
        >
          <option value="issue">issue</option>
          <option value="discussion">discussion</option>
        </select>{' '}
        <label htmlFor="new-label-scope" className="visually-hidden">
          Scope
        </label>
        <select
          id="new-label-scope"
          name="scope"
          value={form.shared ? 'shared' : 'workspace'}
          onChange={(e) => dispatch({ type: 'scope', value: e.target.value })}
        >
          <option value="workspace">this workspace</option>
          <option value="shared">shared (all workspaces)</option>
        </select>{' '}
        <button type="submit" disabled={pending}>
          create label
        </button>
        <span id="new-label-name-error" className="error-msg">
          Name is required.
        </span>
        {errors?.name && (
          <em id="new-label-name-api-error" role="alert">
            {errors.name}
          </em>
        )}
        {errors?.color && (
          <em id="new-label-color-api-error" role="alert">
            {errors.color}
          </em>
        )}
      </form>
      <LabelGroup
        title="Issue labels"
        labels={active.filter((l) => l.kind === 'issue')}
        pending={pending}
        onUpdate={onUpdate}
        onArchive={onArchive}
      />
      <LabelGroup
        title="Discussion labels"
        labels={active.filter((l) => l.kind === 'discussion')}
        pending={pending}
        onUpdate={onUpdate}
        onArchive={onArchive}
      />
      <section>
        <h3>Archived</h3>
        {archived.length === 0 ? (
          <p>none</p>
        ) : (
          <ul>
            {archived.map((l) => (
              <li key={l.id} data-archived="true">
                <LabelBadge label={l} /> <small>{scopeName(l)}</small>{' '}
                <button type="button" disabled={pending} onClick={() => onRestore(l)}>
                  restore
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
