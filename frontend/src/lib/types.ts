// TypeScript mirrors of the JSON shapes in pkg/unpackerr/openapi.json and the
// config section structs. Kept hand-written and small; the OpenAPI doc served at
// /api/openapi.json is the source of truth.

export interface AuthInfo {
  username: string
  apiKey: string
  auth: string
  via: string
  goos: string
  header?: string
  /** Incoming request headers for the proxy-auth picker (already filtered). */
  headers?: Record<string, string[]>
  clientIP?: string
  upstreamAllowed?: boolean
  permissions: string[]
}

export interface Stats {
  waiting: number
  queued: number
  extracting: number
  failed: number
  extracted: number
  imported: number
  deleted: number
  hookOK: number
  hookFail: number
  cmdOK: number
  cmdFail: number
  retries: number
  finished: number
  starrs: number
  folders: number
  webhooks: number
  cmdhooks: number
  stackFS: BufferStat
  stackXtractr: BufferStat
  stackFolder: BufferStat
  stackHook: BufferStat
  stackDel: BufferStat
  stackTask: BufferStat
  starrQueues?: StarrQueueStat[]
}

export interface BufferStat {
  len: number
  cap: number
}

export interface StarrQueueStat {
  app: string
  name: string
  url?: string
  queued: number
  retrieved: number
  complete: number
  match: number
  issues: number
  downloading: number
  updatedAt?: string
  error?: string
}

export interface StarrTestResult {
  queued: number
  retrieved: number
  torrents: number
  nzbs: number
  other?: number
  elapsed: string
}

export interface HookTestResult {
  status: string
  reply?: string
  elapsed: string
}

export interface SystemInfo {
  version: string
  revision: string
  started: string
  uptime: string
  listenAddr: string
  urlbase: string
  auth: string
  metrics: boolean
  configFile: string
  hostname: string
  goos: string
  logs: string
}

export interface QueueItem {
  id: string
  app: string
  url: string
  path: string
  outputPath: string
  status: string
  retries: number
  updated: string
  progress: string
  error: string
  percent?: number
  wrote?: number
  total?: number
  read?: number
  compressed?: number
  files?: number
  count?: number
  archives?: number
  extracted?: number
  archive?: string
  speedBps?: number
  avgSpeedBps?: number
  eta?: string
  due?: string
  dueKind?: 'start' | 'retry' | 'cleanup' | 'history'
  note?: string
  event?: 'fsnotify' | 'polling'
  started?: string
  elapsed?: string
  bytes?: number
  ratio?: number
  queue?: number
  output?: string
  kind?: string
  ids?: Record<string, unknown>
  newFiles?: string[]
  origFiles?: string[]
}

export interface HistoryRecord {
  id: string
  app: string
  url: string
  path: string
  outputPath: string
  status: string
  retries: number
  started: string
  updated: string
  finished: string
  archives: number
  files: number
  bytes: number
  ratio: number
  elapsed: string
  error: string
  progress: string
  kind?: string
  deleteOrig?: boolean
  deleteDelay?: string
  syncthing?: boolean
  splitFlac?: boolean
  maxBytes?: number
  noRetry?: boolean
  newFiles?: string[]
  origFiles?: string[]
  extraFiles?: string[]
  preFiles?: string[]
  forgotten?: boolean
  ids?: Record<string, unknown>
  event?: 'fsnotify' | 'polling'
  queue?: number
  output?: string
}

export type ItemMeta = QueueItem | HistoryRecord

export interface BrowseDir {
  sep: string
  path: string
  mom: string
  dirs: string[]
  files: string[]
  error: string
}

export interface LogFileInfo {
  id: string
  name: string
  path: string
  size: number
  time: string
  mode: string
  used: boolean
  user: string
}

export interface LogFileInfos {
  dirs: string[] | null
  size: number
  list: LogFileInfo[] | null
}

export interface ConfigWriteReply {
  status: string
  restartRequired: boolean
}

// ---- Config sections (file-shaped; the payload you GET and PUT back) ----

export interface GeneralConfig {
  debug: boolean
  quiet: boolean
  activity: boolean
  parallel: number
  errorStderr: boolean
  logFile: string
  logFiles: number
  logFileMb: number
  logFileMode: string
  maxRetries: number
  remnantAction: string
  fileMode: string
  dirMode: string
  logQueues: string
  interval: string
  timeout: string
  deleteDelay: string
  startDelay: string
  retryDelay: string
  progress: string
  keepHistory: number
  passwords: string[]
}

export interface APIKey {
  name: string
  key: string
  roles: string[]
}

export interface Role {
  permissions: string[]
}

export interface WebServer {
  metrics: boolean
  pprof: boolean
  logFiles: number
  logFileMb: number
  listenAddr: string
  logFile: string
  sslCertFile: string
  sslKeyFile: string
  urlbase: string
  upstreams: string[]
  wsOrigins: string[]
  uiPassword: string
  uiRoleHeader: string
  uiCurrentKdf?: string
  apiKeys: APIKey[]
  roles: Record<string, Role> | null
}

export interface StarrConfig {
  name: string
  url: string
  apiKey: string
  httpPass?: string
  httpUser?: string
  username?: string
  password?: string
  path: string
  paths: string[]
  protocols: string
  delete_orig: boolean
  delete_delay: string
  syncthing: boolean
  valid_ssl: boolean
  timeout: string
  maxBytes: string
  split_flac?: boolean
}

export interface FolderConfig {
  path: string
  interval: string
  extract_path: string
  delete_original: boolean
  delete_files: boolean
  disable_log: boolean
  move_back: boolean
  delete_after: string | null
  extract_isos: boolean
  disableRecursion: boolean
  maxNested: number
  extrasMaxDepth: number
  allowSymlinks: boolean
  maxBytes: string
  maxFiles: number
  maxRatio: number
  exclude_paths: string[]
  wait_extensions: string[]
  skip_empty: boolean
}

export interface FoldersSection {
  buffer: number
  folder: Record<string, FolderConfig> | null
}

export interface WebhookConfig {
  name: string
  url: string
  command: string
  contentType: string
  templatePath: string
  template: string
  timeout: string
  shell: boolean
  ignoreSsl: boolean
  silent: boolean
  events: (number | string)[]
  exclude: string[]
  nickname: string
  token: string
  channel: string
}

export type ConfigSection =
  | 'general'
  | 'webserver'
  | 'sonarr'
  | 'radarr'
  | 'lidarr'
  | 'readarr'
  | 'folders'
  | 'webhooks'
  | 'cmdhooks'

export const STARR_SECTIONS: ConfigSection[] = [
  'sonarr',
  'radarr',
  'lidarr',
  'readarr',
]

// Extract statuses used by webhook `events`, matching pkg/unpackerr ExtractStatus.
export const EXTRACT_STATUSES: { value: number; id: string; label: string }[] =
  [
    { value: 0, id: 'waiting', label: 'Waiting' },
    { value: 1, id: 'queued', label: 'Queued' },
    { value: 2, id: 'extracting', label: 'Extracting' },
    { value: 3, id: 'extractfailed', label: 'Extract Failed' },
    { value: 4, id: 'extracted', label: 'Extracted' },
    { value: 5, id: 'imported', label: 'Imported' },
    { value: 6, id: 'deleting', label: 'Deleting' },
    { value: 7, id: 'deletefailed', label: 'Delete Failed' },
    { value: 8, id: 'deleted', label: 'Deleted' },
    { value: 9, id: 'extractednothing', label: 'Nothing Extracted' },
  ]
