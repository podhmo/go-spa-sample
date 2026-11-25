import { api } from '../../../../api'

export const listDiscussionFormData = async (ws: string) => {
  const [categories, labels] = await Promise.all([
    api.listDiscussionCategories(),
    api.listLabels(ws, { kind: 'discussion' }),
  ])
  return { categories: categories.categories, labels: labels.labels }
}
