import { notFound } from '@tanstack/react-router'
import { RequestError } from '../../../api'

export const pullOrNotFound = async <T>(p: Promise<T>): Promise<T> => {
  try {
    return await p
  } catch (err) {
    if (err instanceof RequestError && err.status === 404) throw notFound()
    throw err
  }
}
