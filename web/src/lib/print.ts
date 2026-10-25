export class PopupBlockedError extends Error {
  constructor() {
    super(
      'El navegador bloqueó la ventana de impresión. Permita las ventanas emergentes e intente de nuevo.',
    )
    this.name = 'PopupBlockedError'
  }
}

/**
 * Open one printable window per document. Windows are opened synchronously in
 * the click handler (before any await) so popup blockers see a user gesture;
 * each document is then fetched and sent to window.print().
 */
export async function printDocuments(jobs: Array<() => Promise<string>>): Promise<void> {
  if (jobs.length === 0) return

  const popups: Window[] = []
  const written = new Set<number>()
  for (let i = 0; i < jobs.length; i += 1) {
    const popup = window.open('', '_blank')
    if (!popup) {
      popups.forEach((opened) => opened.close())
      throw new PopupBlockedError()
    }
    popups.push(popup)
  }

  try {
    for (let i = 0; i < jobs.length; i += 1) {
      const html = await jobs[i]()
      const popup = popups[i]
      popup.document.open()
      popup.document.write(html)
      popup.document.close()
      written.add(i)
      if (typeof popup.focus === 'function') popup.focus()
      if (typeof popup.print === 'function') popup.print()
    }
  } catch (error) {
    popups.forEach((popup, index) => {
      if (!written.has(index)) popup.close()
    })
    throw error
  }
}

export function printHtml(job: () => Promise<string>): Promise<void> {
  return printDocuments([job])
}
