// Server-Sent Events, independent of the platform. EventSource cannot send
// an Authorization header, so streams are read by a Transport: fetch
// streaming in browsers, incremental XMLHttpRequest in React Native.

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

/** Opens url and calls onChunk with text as it arrives, until the end. */
export type Transport = (url: string, headers: Record<string, string>, onChunk: (text: string) => void, signal: AbortSignal) => Promise<void>

export const fetchTransport: Transport = async (url, headers, onChunk, signal) => {
  const res = await fetch(url, { headers, signal })
  if (!res.ok || !res.body) throw new Error(`stream ${res.status}`)
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
  for (;;) {
    const { value, done } = await reader.read()
    if (done) return
    onChunk(value)
  }
}

/** For React Native, whose fetch does not stream: responseText grows as data arrives. */
export const xhrTransport: Transport = (url, headers, onChunk, signal) =>
  new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    let seen = 0
    xhr.open('GET', url)
    for (const [k, v] of Object.entries(headers)) xhr.setRequestHeader(k, v)
    xhr.onprogress = () => {
      const text = xhr.responseText
      if (text.length > seen) {
        onChunk(text.slice(seen))
        seen = text.length
      }
    }
    xhr.onload = () => (xhr.status === 200 ? resolve() : reject(new Error(`stream ${xhr.status}`)))
    xhr.onerror = () => reject(new Error('stream error'))
    signal.addEventListener('abort', () => {
      xhr.abort()
      resolve()
    })
    xhr.send()
  })

export interface StreamOptions {
  url: string
  token: () => Promise<string | null>
  onEvent: (e: SSEEvent) => void
  onStatus?: (connected: boolean) => void
  transport?: Transport
  retryMs?: number
}

/** Keeps a stream open, resuming with Last-Event-ID, until the returned function is called. */
export function openStream(o: StreamOptions): () => void {
  const abort = new AbortController()
  const transport = o.transport ?? fetchTransport
  let lastId: string | undefined
  ;(async () => {
    while (!abort.signal.aborted) {
      try {
        const token = await o.token()
        if (!token) return
        const headers: Record<string, string> = { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' }
        if (lastId) headers['Last-Event-ID'] = lastId
        const parser = new SSEParser()
        let connected = false
        await transport(o.url, headers, (chunk) => {
          if (!connected) {
            connected = true
            o.onStatus?.(true)
          }
          for (const ev of parser.push(chunk)) {
            if (ev.id) lastId = ev.id
            o.onEvent(ev)
          }
        }, abort.signal)
      } catch {
        if (abort.signal.aborted) return
      }
      o.onStatus?.(false)
      await new Promise((r) => setTimeout(r, o.retryMs ?? 2000))
    }
  })()
  return () => abort.abort()
}
