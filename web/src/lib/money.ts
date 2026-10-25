/**
 * Money helpers for Bolívares. Every amount is an integer number of cents
 * (unidad mínima). No floating point is used for money arithmetic.
 */

function groupThousands(digits: string): string {
  let out = ''
  for (let i = 0; i < digits.length; i += 1) {
    if (i > 0 && (digits.length - i) % 3 === 0) {
      out += '.'
    }
    out += digits[i]
  }
  return out
}

/** Format an integer amount of cents as `Bs 1.234,56`. */
export function formatBs(amount: number): string {
  if (!Number.isSafeInteger(amount)) {
    throw new Error('El monto debe ser un entero en unidad mínima')
  }
  const negative = amount < 0
  const absolute = Math.abs(amount)
  const units = Math.floor(absolute / 100)
  const cents = absolute % 100
  const formatted = `${groupThousands(String(units))},${String(cents).padStart(2, '0')}`
  return `${negative ? '-' : ''}Bs ${formatted}`
}

function invalid(): never {
  throw new Error('Monto inválido')
}

/**
 * Parse a user-entered Bs amount into integer cents.
 *
 * Accepted: `1.234,56`, `1234,56`, `1234.56`, `1234` (whole bolívares).
 * A lone dot followed by two digits is a decimal separator; otherwise a dot is
 * a thousands separator. More than two decimals, negative values, letters and
 * misplaced separators are rejected.
 */
export function parseBs(raw: string): number {
  let value = raw.trim()
  if (value.toLowerCase().startsWith('bs')) {
    value = value.slice(2).trim()
  }
  if (value === '') invalid()
  if (value.startsWith('-') || value.includes('+') || /\s/.test(value)) invalid()
  if (!/^[0-9.,]+$/.test(value)) invalid()

  const lastDot = value.lastIndexOf('.')
  const lastComma = value.lastIndexOf(',')

  let integerPart: string
  let decimalPart = ''

  if (lastComma >= 0) {
    if (value.indexOf(',') !== lastComma) invalid()
    integerPart = value.slice(0, lastComma)
    decimalPart = value.slice(lastComma + 1)
    if (decimalPart === '') invalid()
    // Any remaining dot is a thousands separator.
    if (integerPart.includes('.')) {
      if (!/^\d{1,3}(\.\d{3})+$/.test(integerPart)) invalid()
      integerPart = integerPart.replaceAll('.', '')
    }
  } else if (lastDot >= 0) {
    const dotCount = (value.match(/\./g) ?? []).length
    const afterDot = value.slice(lastDot + 1)
    if (dotCount === 1 && afterDot.length > 0 && afterDot.length <= 2) {
      // Decimal dot: 1234.56
      integerPart = value.slice(0, lastDot)
      decimalPart = afterDot
    } else {
      // Thousands dot: 1.234 or 1.234.567
      if (!/^\d{1,3}(\.\d{3})+$/.test(value)) invalid()
      integerPart = value.replaceAll('.', '')
    }
  } else {
    integerPart = value
  }

  if (!/^\d+$/.test(integerPart)) invalid()
  if (!/^\d*$/.test(decimalPart)) invalid()
  if (decimalPart.length > 2) invalid()

  const units = Number(integerPart)
  if (!Number.isSafeInteger(units)) invalid()
  const cents = decimalPart === '' ? 0 : Number(decimalPart.padEnd(2, '0'))
  return units * 100 + cents
}
