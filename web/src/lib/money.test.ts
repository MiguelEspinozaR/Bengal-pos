import { describe, expect, it } from 'vitest'
import { formatBs, parseBs } from './money'

describe('formatBs', () => {
  it('formatea enteros de unidad mínima sin flotantes', () => {
    expect(formatBs(123456)).toBe('Bs 1.234,56')
    expect(formatBs(0)).toBe('Bs 0,00')
    expect(formatBs(5)).toBe('Bs 0,05')
    expect(formatBs(100)).toBe('Bs 1,00')
    expect(formatBs(123456789)).toBe('Bs 1.234.567,89')
    expect(formatBs(-123456)).toBe('-Bs 1.234,56')
  })

  it('rechaza montos no enteros', () => {
    expect(() => formatBs(1.5)).toThrow()
  })
})

describe('parseBs', () => {
  it('acepta los formatos documentados', () => {
    expect(parseBs('1.234,56')).toBe(123456)
    expect(parseBs('1234,56')).toBe(123456)
    expect(parseBs('1234.56')).toBe(123456)
    expect(parseBs('1234')).toBe(123400)
  })

  it('acepta coma o punto decimal y separadores de miles', () => {
    expect(parseBs('0,05')).toBe(5)
    expect(parseBs('0.05')).toBe(5)
    expect(parseBs('1.234')).toBe(123400)
    expect(parseBs('Bs 1.234,56')).toBe(123456)
    expect(parseBs(' 12,3 ')).toBe(1230)
  })

  it('rechaza entradas inválidas, negativas o con más de dos decimales', () => {
    expect(() => parseBs('')).toThrow()
    expect(() => parseBs('abc')).toThrow()
    expect(() => parseBs('-1')).toThrow()
    expect(() => parseBs('1,234')).toThrow()
    expect(() => parseBs('1.2345')).toThrow()
    expect(() => parseBs('1,2,3')).toThrow()
    expect(() => parseBs('1,')).toThrow()
    expect(() => parseBs('1 234')).toThrow()
  })
})
