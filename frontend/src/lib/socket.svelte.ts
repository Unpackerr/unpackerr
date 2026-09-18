import { api, getUrlbase } from './api'
import { appError } from './toast'
import { router } from './router.svelte'
import { isFinishedHistory } from './format'
import type { HistoryRecord, QueueItem, Stats } from './types'

export type LiveTopic = 'queue' | 'progress' | 'history' | 'logs' | 'errors'

type WsFrame = {
  type: string
  topics?: string[]
  payload?: any
  file?: string
  line?: string
  error?: string
}

function wsURL(): string {
  const base = getUrlbase().replace(/\/+$/, '')
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${location.host}${base}/ws`
}

type LiveGlobal = typeof globalThis & {
  __unpackerrLive?: LiveSocket
  __unpackerrLiveSession?: boolean
}

const g = globalThis as LiveGlobal

class LiveSocket {
  connected = $state(false)
  stats = $state.raw<Stats | null>(null)
  queue = $state.raw<QueueItem[]>([])
  history = $state.raw<HistoryRecord[]>([])
  fetchedAt = $state<number | undefined>(undefined)

  private ws: WebSocket | null = null
  private wanted = new Set<LiveTopic>(['errors'])
  private logFile = ''
  private timer: ReturnType<typeof setTimeout> | undefined
  private onLog: ((file: string, line: string) => void) | undefined
  private onErrorLine: ((line: string) => void) | undefined
  private closed = true
  /** Bumped on live frames, socket stop, and reconnect so in-flight REST cannot overwrite newer state. */
  private liveSeq = 0

  connect() {
    g.__unpackerrLiveSession = true
    this.closed = false
    this.stopSocket()
    this.open()
  }

  disconnect() {
    g.__unpackerrLiveSession = false
    this.closed = true
    this.stopSocket()
  }

  /** Close without clearing the login session; used when Vite replaces this module. */
  abandon() {
    this.closed = true
    this.stopSocket()
  }

  takeOver(prev: LiveSocket) {
    this.wanted = new Set(prev.wanted)
    this.logFile = prev.logFile
    this.onLog = prev.onLog
    this.onErrorLine = prev.onErrorLine
    this.stats = prev.stats
    this.queue = prev.queue
    this.history = prev.history
    this.fetchedAt = prev.fetchedAt
    this.liveSeq = prev.liveSeq
  }

  subscribe(topics: LiveTopic[], file?: string) {
    for (const topic of topics) this.wanted.add(topic)
    if (file) this.logFile = file
    this.sendSub()
  }

  unsubscribe(topics: LiveTopic[]) {
    for (const topic of topics) this.wanted.delete(topic)
    if (topics.includes('logs')) this.logFile = ''
    this.send({ op: 'unsub', topics })
  }

  setLogHandler(fn?: (file: string, line: string) => void) {
    this.onLog = fn
  }

  setErrorHandler(fn?: (line: string) => void) {
    this.onErrorLine = fn
  }

  private stopSocket() {
    this.liveSeq++
    if (this.timer) {
      clearTimeout(this.timer)
      this.timer = undefined
    }
    if (this.ws) {
      this.ws.onclose = null
      this.ws.onmessage = null
      this.ws.onopen = null
      this.ws.close()
      this.ws = null
    }
    this.connected = false
  }

  private open() {
    if (this.closed || this.ws) return
    const socket = new WebSocket(wsURL())
    this.ws = socket
    socket.onopen = () => {
      this.connected = true
      this.sendSub()
    }
    socket.onmessage = (ev) => this.handle(String(ev.data))
    socket.onclose = () => {
      this.connected = false
      this.ws = null
      this.liveSeq++
      if (!this.closed) this.timer = setTimeout(() => this.open(), 1500)
    }
  }

  private sendSub() {
    const topics = [...this.wanted]
    if (!topics.length) return
    this.send({ op: 'sub', topics, file: this.logFile })
  }

  private send(msg: object) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(msg))
  }

  private handle(raw: string) {
    let frame: WsFrame
    try {
      frame = JSON.parse(raw)
    } catch {
      return
    }

    switch (frame.type) {
      case 'hello':
        void this.restSnapshot()
        break
      case 'queue':
        if (frame.payload?.stats) this.stats = frame.payload.stats
        if (Array.isArray(frame.payload?.items)) this.queue = frame.payload.items
        this.bumpLive()
        break
      case 'progress':
        this.applyProgress(frame.payload)
        this.bumpLive()
        break
      case 'history':
        this.applyHistory(frame.payload)
        this.bumpLive()
        break
      case 'log':
        this.onLog?.(frame.file ?? '', frame.line ?? '')
        break
      case 'error': {
        const msg = frame.payload?.msg
        if (!msg) break
        const at = frame.payload?.at
        const line = at ? `${at} ${msg}` : String(msg)
        this.onErrorLine?.(line)
        if (!this.onErrorLine && router.path !== '/logs/errors') {
          appError(String(msg))
        }
        break
      }
    }
  }

  private applyProgress(item: QueueItem) {
    if (!item?.id) return
    const idx = this.queue.findIndex((row) => row.id === item.id)
    if (idx < 0) {
      this.queue = [...this.queue, item]
      return
    }
    // Replace the row. A shallow merge keeps stale due/speed when Go omits zeros.
    const next = this.queue.slice()
    next[idx] = item
    this.queue = next
  }

  private applyHistory(payload: {
    op?: string
    rows?: HistoryRecord[]
    row?: HistoryRecord
    id?: string
  }) {
    if (!payload) return
    if (payload.op === 'snapshot' && Array.isArray(payload.rows)) {
      this.history = payload.rows.filter(isFinishedHistory)
      return
    }
    if (payload.op === 'clear') {
      this.history = []
      return
    }
    if (payload.op === 'delete' && payload.id) {
      this.history = this.history.filter((row) => row.id !== payload.id)
      return
    }
    if (payload.op === 'upsert' && payload.row) {
      const row = payload.row
      if (!isFinishedHistory(row)) {
        this.history = this.history.filter((item) => item.id !== row.id)
        return
      }
      const idx = this.history.findIndex((item) => item.id === row.id)
      if (idx < 0) {
        this.history = [row, ...this.history]
        return
      }
      const next = this.history.slice()
      next[idx] = row
      this.history = next
    }
  }

  private bumpLive() {
    this.liveSeq++
    this.fetchedAt = Date.now()
  }

  async restSnapshot() {
    const seq = this.liveSeq
    const [stats, queue, history] = await Promise.all([
      api.get<Stats>('stats'),
      api.get<QueueItem[]>('queue'),
      api.get<HistoryRecord[]>('history'),
    ])
    if (seq !== this.liveSeq) return
    if (stats.ok) this.stats = stats.body
    if (queue.ok) this.queue = queue.body ?? []
    if (history.ok) this.history = (history.body ?? []).filter(isFinishedHistory)
    if (stats.ok || queue.ok || history.ok) this.fetchedAt = Date.now()
  }
}

const prev = g.__unpackerrLive
if (prev) prev.abandon()

export const live = new LiveSocket()
if (prev) live.takeOver(prev)
g.__unpackerrLive = live

if (g.__unpackerrLiveSession) live.connect()
