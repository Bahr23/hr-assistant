export function moveItem<T>(items: readonly T[], fromIndex: number, toIndex: number): T[] {
  if (fromIndex === toIndex || fromIndex < 0 || toIndex < 0 || fromIndex >= items.length || toIndex >= items.length) {
    return [...items]
  }

  const next = [...items]
  const moved = next.splice(fromIndex, 1)[0] as T
  next.splice(toIndex, 0, moved)
  return next
}
