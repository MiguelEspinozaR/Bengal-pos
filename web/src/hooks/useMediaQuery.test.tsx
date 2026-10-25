import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MOBILE_QUERY, useIsMobile } from './useMediaQuery'

describe('useIsMobile', () => {
  it('lee y reacciona al breakpoint de 768 px', () => {
    let listener: (() => void) | null = null
    const media = {
      matches: false,
      media: MOBILE_QUERY,
      onchange: null,
      addEventListener: (_event: string, callback: () => void) => {
        listener = callback
      },
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }
    const matchMedia = vi.fn(() => media)
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      writable: true,
      value: matchMedia,
    })

    const { result } = renderHook(() => useIsMobile())
    expect(matchMedia).toHaveBeenCalledWith(MOBILE_QUERY)
    expect(result.current).toBe(false)

    act(() => {
      media.matches = true
      listener?.()
    })
    expect(result.current).toBe(true)
  })
})
