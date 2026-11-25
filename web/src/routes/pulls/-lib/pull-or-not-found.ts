import { notFound } from '@tanstack/react-router'
import { RequestError, api } from '../../../api'

export const pullOrNotFound = async <T>(p: Promise<T>): Promise<T> => {
  try {
    return await p
  } catch (err) {
    if (err instanceof RequestError && err.status === 404) throw notFound()
    throw err
  }
}

export const getPullOrNotFound = (pullId: number) => pullOrNotFound(api.getPull(pullId))

export const getPullFilesOrNotFound = (pullId: number) =>
  pullOrNotFound(
    Promise.all([api.getPull(pullId), api.getPullDiff(pullId)]).then(([detail, diff]) => ({
      ...detail,
      diff,
    })),
  )
