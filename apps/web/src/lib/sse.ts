// Server-Sent Events over fetch: EventSource cannot send an Authorization
// header. Reconnects with Last-Event-ID so nothing is missed.

export interface SSEEvent {
  id?: string
  event: string
  data: string
}

/** Splits a text stream into events (the SSE wire format). */
export class SSEParser {
  private buffer = ''
  private event: Partial<SSEEvent> & { data?: string } = {}

  push(chunk: string): SSEEvent[] {
    this.buffer += chunk.replace(/\r\n?/g, '\n')
    const out: SSEEvent[] = []
    let nl: number
    while ((nl = this.buffer.indexOf('\n')) >= 0) {
      const line = this.buffer.slice(0, nl)
      this.buffer = this.buffer.slice(nl + 1)
      if (line === '') {
        if (this.event.data !== undefined) {
          out.push({ id: this.event.id, event: this.event.event ?? 'message', data: this.event.data })
        }
        this.event = {}
        continue
      }
      if (line.startsWith(':')) continue // comment (keep-alive ping)
      const colon = line.indexOf(':')
      const field = colon < 0 ? line : line.slice(0, colon)
      const value = colon < 0 ? '' : line.slice(colon + 1).replace(/^ /, '')
      if (field === 'data') this.event.data = this.event.data === undefined ? value : `${this.event.data}\n${value}`
      else if (field === 'event') this.event.event = value
      else if (field === 'id') this.event.id = value
    }
    return out
  }
}

export interface StreamOptions {
  url: string
  token: () => Promise<string | null>
  onEvent: (e: SSEEvent) => void
  onStatus?: (connected: boolean) => void
  retryMs?: number
}

/** Keeps a stream open until the returned function is called. */
export function openStream(o: StreamOptions): () => void {
  const abort = new AbortController()
  let lastId: string | undefined
  const retry = o.retryMs ?? 2000
  ;(async () => {
    while (!abort.signal.aborted) {
      try {
        const token = await o.token()
        if (!token) return
        const headers: Record<string, string> = { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' }
        if (lastId) headers['Last-Event-ID'] = lastId
        const res = await fetch(o.url, { headers, signal: abort.signal })
        if (!res.ok || !res.body) throw new Error(`stream ${res.status}`)
        o.onStatus?.(true)
        const parser = new SSEParser()
        const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
        for (;;) {
          const { value, done } = await reader.read()
          if (done) break
          for (const ev of parser.push(value)) {
            if (ev.id) lastId = ev.id
            o.onEvent(ev)
          }
        }
      } catch {
        if (abort.signal.aborted) return
      }
      o.onStatus?.(false)
      await new Promise((r) => setTimeout(r, retry))
    }
  })()
  return () => abort.abort()
}
