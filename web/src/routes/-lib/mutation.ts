import { RequestError } from '../../api'

// Maps a failed mutation to the per-field error map the API returns for
// 422 ({"error":{"fields":{...}}}), falling back to a generic error under
// `field`.
export const apiFieldErrors = (err: unknown, field = 'title'): Record<string, string> =>
  err instanceof RequestError && err.apiError?.fields
    ? err.apiError.fields
    : { [field]: err instanceof Error ? err.message : String(err) }
