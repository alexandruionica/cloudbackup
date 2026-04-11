export type JobState =
  | 'running' | 'stopping' | 'stopped'
  | 'started' | 'finished' | 'failed' | 'cancelled' | 'crashed';

export interface StatsCounters {
  examined_files?: number;
  examined_directories?: number;
  examined_symlinks?: number;
  uploaded_files?: number;
  uploaded_directories?: number;
  uploaded_symlinks?: number;
  failed_to_upload_files?: number;
  up_to_date_files?: number;
  up_to_date_directories?: number;
  excluded?: number;
  [k: string]: number | undefined;
}

export interface ObjectStoreRates {
  name: string;
  type: string;
  rate_1min?: number;
  rate_5min?: number;
  rate_15min?: number;
}

export interface BackupJobStatus {
  name: string;
  state: JobState;
  start_time?: string;
  end_time?: string;
  platform?: string;
  job_id?: string;
  next_run?: string;
  file_content_bytes_read?: number;
  rate_1min?: number;
  rate_5min?: number;
  rate_15min?: number;
  stats_counters?: StatsCounters;
  stats_text?: { current_directory?: string; current_file?: string };
  object_store_states?: ObjectStoreRates[];
}

export interface WatchEvent {
  sequence: number;
  name: string;
  percent_done: number;
  rate: number;
  type: 'file' | 'directory' | 'symlink' | 'unknown';
  store_name: string;
  store_type: string;
  operation_type: 'excluded' | 'examine' | 'upload' | 'metadata' | 'up_to_date' | 'mark_deleted';
  error: string;
}

export interface ApiResponse<T> {
  code: string;
  message: string;
  result?: T;
}

export interface ResultBackupJobStart {
  name: string;
  job_id: string;
}

export interface Connection {
  baseUrl: string;
  username: string;
  password: string;
}

function authHeader(c: Connection): Record<string, string> {
  if (!c.username && !c.password) return {};
  return { Authorization: 'Basic ' + btoa(`${c.username}:${c.password}`) };
}

function url(c: Connection, path: string): string {
  const base = (c.baseUrl || '').replace(/\/+$/, '');
  return `${base}/api/v1${path}`;
}

async function jsonRequest<T>(c: Connection, path: string, method: string, body?: unknown): Promise<T> {
  const res = await fetch(url(c, path), {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...authHeader(c),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json: any = {};
  if (text) {
    try { json = JSON.parse(text); }
    catch { throw new Error(`Invalid JSON (HTTP ${res.status}): ${text.slice(0, 200)}`); }
  }
  if (!res.ok || (json && json.code === 'error')) {
    throw new Error(json?.message || `HTTP ${res.status}`);
  }
  return json as T;
}

export async function listBackups(c: Connection): Promise<BackupJobStatus[]> {
  const r = await jsonRequest<ApiResponse<BackupJobStatus[]>>(c, '/backup/list', 'GET');
  return r.result ?? [];
}

export async function startBackup(c: Connection, name: string): Promise<ResultBackupJobStart> {
  const r = await jsonRequest<ApiResponse<ResultBackupJobStart>>(c, '/backup/start', 'POST', { name });
  if (!r.result) throw new Error('Server returned no result');
  return r.result;
}

export async function stopBackup(c: Connection, name: string, jobId?: string): Promise<void> {
  const body: Record<string, string> = { name };
  if (jobId) body.job_id = jobId;
  await jsonRequest<ApiResponse<unknown>>(c, '/backup/stop', 'POST', body);
}

export async function getVersion(c: Connection): Promise<ApiResponse<unknown>> {
  return jsonRequest<ApiResponse<unknown>>(c, '/report/version', 'GET');
}

/**
 * Stream live watch events for a job. Uses fetch() streaming because the native
 * EventSource cannot POST a request body nor send an Authorization header.
 * Returns a function that cancels the stream.
 */
export function watchBackup(
  c: Connection,
  name: string,
  jobId: string,
  onEvent: (e: WatchEvent) => void,
  onClose: (reason: string) => void,
  onError: (err: Error) => void,
): () => void {
  const ctrl = new AbortController();
  (async () => {
    try {
      const res = await fetch(url(c, '/backup/watch'), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Accept: 'text/event-stream',
          ...authHeader(c),
        },
        body: JSON.stringify({ name, job_id: jobId }),
        signal: ctrl.signal,
      });
      if (!res.ok || !res.body) {
        const text = await res.text().catch(() => '');
        throw new Error(`HTTP ${res.status}${text ? `: ${text.slice(0, 200)}` : ''}`);
      }
      // The CloudBackup server emits one SSE event per line as
      // "data: <json>\n" (single LF, not the standard "\n\n" event
      // terminator), matching what the CLI client in
      // client/backup/backup.go parses with ReadBytes('\n'). So we split
      // on '\n' and treat each "data:" line as a complete event.
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      const handleLine = (rawLine: string): boolean => {
        const line = rawLine.replace(/\r$/, '');
        if (!line.startsWith('data:')) return false;
        const data = line.replace(/^data:\s?/, '');
        if (!data) return false;
        if (data.startsWith('Backup job ') || data.startsWith('Completed run')) {
          onClose(data);
          return true;
        }
        try { onEvent(JSON.parse(data) as WatchEvent); }
        catch { /* ignore non-JSON payloads */ }
        return false;
      };
      while (true) {
        const { done, value } = await reader.read();
        if (done) {
          if (buf && handleLine(buf)) return;
          onClose('stream ended');
          return;
        }
        buf += decoder.decode(value, { stream: true });
        let nl: number;
        while ((nl = buf.indexOf('\n')) !== -1) {
          const line = buf.slice(0, nl);
          buf = buf.slice(nl + 1);
          if (handleLine(line)) return;
        }
      }
    } catch (e: any) {
      if (e?.name === 'AbortError') return;
      onError(e instanceof Error ? e : new Error(String(e)));
    }
  })();
  return () => ctrl.abort();
}
