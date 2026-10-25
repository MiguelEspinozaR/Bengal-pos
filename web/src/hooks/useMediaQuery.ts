import { useEffect, useState } from 'react'

/**
 * Small matchMedia wrapper used to isolate the responsive breakpoint from the
 * layout. jsdom tests mock matchMedia; production uses the real media query.
 */
export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState<boolean>(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return false
    }
    return window.matchMedia(query).matches
  })

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return
    }
    const media = window.matchMedia(query)
    const update = () => setMatches(media.matches)
    update()
    if (typeof media.addEventListener === 'function') {
      media.addEventListener('change', update)
      return () => media.removeEventListener('change', update)
    }
    // Safari < 14 fallback.
    media.addListener(update)
    return () => media.removeListener(update)
  }, [query])

  return matches
}

export const MOBILE_QUERY = '(max-width: 768px)'

export function useIsMobile(): boolean {
  return useMediaQuery(MOBILE_QUERY)
}
