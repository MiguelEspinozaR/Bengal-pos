import { describe, expect, it } from 'vitest'
import { matchesProduct } from './text'

describe('búsqueda de productos', () => {
  const producto = { codigo: 'CAF-01', nombre: 'Café con leche' }

  it('ignora mayúsculas y acentos en nombre y código', () => {
    expect(matchesProduct(producto, 'cafe')).toBe(true)
    expect(matchesProduct(producto, 'CAFÉ')).toBe(true)
    expect(matchesProduct(producto, 'caf-01')).toBe(true)
    expect(matchesProduct(producto, '  leche ')).toBe(true)
  })

  it('no coincide con texto ausente y acepta búsqueda vacía', () => {
    expect(matchesProduct(producto, 'té')).toBe(false)
    expect(matchesProduct(producto, '')).toBe(true)
  })
})
