/** Lowercase and remove accents so product search is forgiving. */
export function normalizeText(value: string): string {
  return value
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .trim()
}

export function matchesProduct(
  producto: { codigo: string; nombre: string },
  query: string,
): boolean {
  const needle = normalizeText(query)
  if (needle === '') return true
  return (
    normalizeText(producto.codigo).includes(needle) ||
    normalizeText(producto.nombre).includes(needle)
  )
}
