import { describe, expect, it } from 'vitest'
import { SSEParser } from './sse'

describe('SSEParser', () => {
  it('parses events split across chunks, skipping pings', () => {
    const p = new SSEParser()
    expect(p.push('retry: 2000\n\n: ping\n\nid: 1\nevent: notif')).toEqual([])
    expect(p.push('ication\ndata: {"a":1}\n\n')).toEqual([{ id: '1', event: 'notification', data: '{"a":1}' }])
  })
  it('joins multi-line data and handles CRLF', () => {
    const p = new SSEParser()
    expect(p.push('data: one\r\ndata: two\r\n\r\n')).toEqual([{ id: undefined, event: 'message', data: 'one\ntwo' }])
  })
})
