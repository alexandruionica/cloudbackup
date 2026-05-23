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
  // client-side-encryption counters
  skipped_reserved_path?: number;
  skipped_too_large_for_target?: number;
  keystore_inconsistent?: number;
  decrypt_keystore_mismatch?: number;
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
  job_type?: 'backup' | 'restore' | '';
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

export interface ReportListItem {
  name: string;
  job_id: string;
  start_time: string;
  end_time: string;
  state: string;
}

interface ReportListApiResponse {
  code: string;
  message: string;
  next?: string;
  result?: ReportListItem[];
}

export interface ReportListPage {
  items: ReportListItem[];
  next: string;
}

export async function listReports(c: Connection, name: string, next?: string): Promise<ReportListPage> {
  const body: Record<string, unknown> = { name };
  if (next) body.next = next;
  const r = await jsonRequest<ReportListApiResponse>(c, '/report/backup/list', 'POST', body);
  return { items: r.result ?? [], next: r.next ?? '' };
}

export async function showReport(c: Connection, name: string, jobId: string): Promise<BackupJobStatus> {
  const r = await jsonRequest<ApiResponse<BackupJobStatus>>(
    c, '/report/backup/show', 'POST', { name, job_id: jobId },
  );
  if (!r.result) throw new Error('Server returned no result');
  return r.result;
}

export interface ServerVersion {
  CloudBackup?: string;
  BuildDate?: string;
  OS?: string;
  Arch?: string;
  Runtime?: string;
  AwsSdk?: string;
  GcpStorageSdk?: string;
  AzureBlobStorageSdk?: string;
}

export async function getVersion(c: Connection): Promise<ServerVersion | null> {
  // The server returns result as a single object despite what the
  // swagger doc declares (httpd/misc_handlers.go:handlerVersion passes
  // the struct directly to JSONSuccessWithResult, not wrapped in an
  // array).
  const r = await jsonRequest<ApiResponse<ServerVersion>>(c, '/report/version', 'GET');
  return r.result ?? null;
}

// ---------------------------------------------------------------------------
// Restore endpoints
// ---------------------------------------------------------------------------

export interface RestoreStartInput {
  name: string;
  source_backup_job_id: string;
  target_name?: string;
  files?: string[];
  all_files?: boolean;
  restore_dir?: string;
  exclusions?: string[];
}

export interface ResultRestoreJobStart {
  name: string;
  restore_job_id: string;
}

export async function listRestores(c: Connection): Promise<BackupJobStatus[]> {
  const r = await jsonRequest<ApiResponse<BackupJobStatus[]>>(c, '/restore/list', 'GET');
  return r.result ?? [];
}

export async function startRestore(c: Connection, input: RestoreStartInput): Promise<ResultRestoreJobStart> {
  const r = await jsonRequest<ApiResponse<ResultRestoreJobStart>>(c, '/restore/start', 'POST', input);
  if (!r.result) throw new Error('Server returned no result');
  return r.result;
}

export async function stopRestore(c: Connection, name: string, restoreJobId?: string): Promise<void> {
  const body: Record<string, string> = { name };
  if (restoreJobId) body.restore_job_id = restoreJobId;
  await jsonRequest<ApiResponse<unknown>>(c, '/restore/stop', 'POST', body);
}

export async function resumeRestore(
  c: Connection, name: string, targetName: string, restoreJobId: string,
): Promise<ResultRestoreJobStart> {
  const r = await jsonRequest<ApiResponse<ResultRestoreJobStart>>(c, '/restore/resume', 'POST', {
    name, target_name: targetName, restore_job_id: restoreJobId,
  });
  if (!r.result) throw new Error('Server returned no result');
  return r.result;
}

export async function listRestoreReports(c: Connection, name: string, next?: string): Promise<ReportListPage> {
  const body: Record<string, unknown> = { name };
  if (next) body.next = next;
  const r = await jsonRequest<ReportListApiResponse>(c, '/report/restore/list', 'POST', body);
  return { items: r.result ?? [], next: r.next ?? '' };
}

export async function showRestoreReport(c: Connection, name: string, jobId: string): Promise<BackupJobStatus> {
  const r = await jsonRequest<ApiResponse<BackupJobStatus>>(
    c, '/report/restore/show', 'POST', { name, job_id: jobId },
  );
  if (!r.result) throw new Error('Server returned no result');
  return r.result;
}

// File-list browsing for picking restore selections. Mirrors the per-directory
// pagination semantics used by client/restore/browse.go.
export interface FileListInstance {
  job_id: string;
  job_name: string;
  job_start_time: string;
  target: string;
  size: number;
  type: 'file' | 'directory' | 'symlink' | string;
  upload_date: string;
  deleted: boolean;
}

export interface FileListEntry {
  path: string;
  parent: string;
  instances: FileListInstance[];
}

interface FileListApiResponse {
  code: string;
  message: string;
  next?: string;
  result?: FileListEntry[];
}

export interface FileListPage {
  items: FileListEntry[];
  next: string;
}

export async function listBackupFiles(
  c: Connection,
  name: string,
  jobId: string,
  path: string,
  descend: boolean,
  next?: string,
): Promise<FileListPage> {
  const body: Record<string, unknown> = { name, job_id: jobId, path, descend };
  if (next) body.next = next;
  const r = await jsonRequest<FileListApiResponse>(c, '/report/backup/file/list', 'POST', body);
  return { items: r.result ?? [], next: r.next ?? '' };
}

// Server config — used by the restore UI to learn the list of targets defined
// for a given backup definition (needed both for target selection at start
// time and for resume which requires target_name).
export interface ConfigBackupTarget {
  name: string;
  type?: string;
  [k: string]: unknown;
}

export interface ConfigBackup {
  name: string;
  target?: ConfigBackupTarget[];
  [k: string]: unknown;
}

export interface FullConfig {
  backup?: ConfigBackup[];
  [k: string]: unknown;
}

export async function getConfig(c: Connection): Promise<FullConfig | null> {
  const r = await jsonRequest<ApiResponse<FullConfig>>(c, '/config', 'GET');
  return r.result ?? null;
}

/**
 * Stream live watch events from one of the SSE endpoints. The cloudbackup
 * server emits "data: <json>\n" lines (single LF, not the standard "\n\n"
 * SSE terminator). Trailing terminator messages start with "Backup job ",
 * "Restore job " or "Completed run".
 */
function streamSseEvents(
  c: Connection,
  path: string,
  body: Record<string, unknown>,
  onEvent: (e: WatchEvent) => void,
  onClose: (reason: string) => void,
  onError: (err: Error) => void,
): () => void {
  const ctrl = new AbortController();
  (async () => {
    try {
      const res = await fetch(url(c, path), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Accept: 'text/event-stream',
          ...authHeader(c),
        },
        body: JSON.stringify(body),
        signal: ctrl.signal,
      });
      if (!res.ok || !res.body) {
        const text = await res.text().catch(() => '');
        throw new Error(`HTTP ${res.status}${text ? `: ${text.slice(0, 200)}` : ''}`);
      }
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';
      const handleLine = (rawLine: string): boolean => {
        const line = rawLine.replace(/\r$/, '');
        if (!line.startsWith('data:')) return false;
        const data = line.replace(/^data:\s?/, '');
        if (!data) return false;
        if (data.startsWith('Backup job ') || data.startsWith('Restore job ') || data.startsWith('Completed run')) {
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

export function watchBackup(
  c: Connection,
  name: string,
  jobId: string,
  onEvent: (e: WatchEvent) => void,
  onClose: (reason: string) => void,
  onError: (err: Error) => void,
): () => void {
  return streamSseEvents(c, '/backup/watch', { name, job_id: jobId }, onEvent, onClose, onError);
}

export function watchRestore(
  c: Connection,
  name: string,
  restoreJobId: string,
  onEvent: (e: WatchEvent) => void,
  onClose: (reason: string) => void,
  onError: (err: Error) => void,
): () => void {
  return streamSseEvents(
    c, '/restore/watch',
    { name, restore_job_id: restoreJobId },
    onEvent, onClose, onError,
  );
}
