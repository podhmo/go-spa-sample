import { api } from '../../../../api'

export const listIssueFormLabels = (ws: string) => api.listLabels(ws, { kind: 'issue' })
