import { html } from 'htm/preact';
import { render } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  getVersion, listBackups, listReports, showReport, startBackup, stopBackup, watchBackup,
  type BackupJobStatus, type Connection, type ReportListItem, type ServerVersion, type WatchEvent,
} from './api.js';

const STORAGE_KEY = 'cloudbackup.connection';
const POLL_MS = 3000;
const MAX_EVENTS = 500;

function defaultConnection(): Connection {
  const base = location.protocol === 'file:'
    ? 'http://127.0.0.1:8080'
    : `${location.protocol}//${location.host}`;
  return { baseUrl: base, username: '', password: '' };
}

function loadConnection(): Connection {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed.baseUrl === 'string') return parsed as Connection;
    }
  } catch { /* ignore */ }
  return defaultConnection();
}

function saveConnection(c: Connection): void {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(c)); } catch { /* ignore */ }
}

function fmtBytes(n?: number): string {
  if (n === undefined || n === null || isNaN(n)) return '-';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = n, u = 0;
  while (v >= 1024 && u < units.length - 1) { v /= 1024; u++; }
  return `${u === 0 ? v.toFixed(0) : v.toFixed(1)} ${units[u]}`;
}

function fmtTime(s?: string): string {
  if (!s || s.startsWith('0001-01-01')) return '-';
  const d = new Date(s);
  if (isNaN(d.getTime())) return s;
  return d.toLocaleString();
}

function fmtDuration(start?: string, end?: string): string {
  if (!start || start.startsWith('0001-01-01')) return '-';
  const startMs = new Date(start).getTime();
  const endMs = end && !end.startsWith('0001-01-01') ? new Date(end).getTime() : NaN;
  if (isNaN(startMs) || isNaN(endMs)) return '-';
  let s = Math.max(0, Math.floor((endMs - startMs) / 1000));
  const h = Math.floor(s / 3600); s -= h * 3600;
  const m = Math.floor(s / 60); s -= m * 60;
  if (h) return `${h}h${m}m${s}s`;
  if (m) return `${m}m${s}s`;
  return `${s}s`;
}

function isRunning(state: string): boolean {
  return state === 'running' || state === 'started' || state === 'stopping';
}

function App() {
  const [conn, setConn] = useState<Connection>(loadConnection);
  const [jobs, setJobs] = useState<BackupJobStatus[]>([]);
  const [version, setVersion] = useState<ServerVersion | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [connOpen, setConnOpen] = useState(false);
  const [watchTarget, setWatchTarget] = useState<BackupJobStatus | null>(null);
  const [reportsFor, setReportsFor] = useState<BackupJobStatus | null>(null);
  const [busy, setBusy] = useState<Record<string, boolean>>({});

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const list = await listBackups(conn);
      setJobs(list);
      setError(null);
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setLoading(false);
    }
  }, [conn]);

  useEffect(() => {
    let stopped = false;
    const tick = async () => {
      if (stopped) return;
      await refresh();
    };
    void tick();
    const id = setInterval(() => { void tick(); }, POLL_MS);
    return () => { stopped = true; clearInterval(id); };
  }, [refresh]);

  useEffect(() => {
    let stopped = false;
    setVersion(null);
    (async () => {
      try {
        const v = await getVersion(conn);
        if (!stopped) setVersion(v);
      } catch {
        // version is a best-effort display; surface any auth/network
        // trouble through the main error channel from listBackups.
      }
    })();
    return () => { stopped = true; };
  }, [conn]);

  const onStart = useCallback(async (name: string) => {
    setBusy(b => ({ ...b, [name]: true }));
    try { await startBackup(conn, name); await refresh(); }
    catch (e: any) { setError(e?.message || String(e)); }
    finally { setBusy(b => ({ ...b, [name]: false })); }
  }, [conn, refresh]);

  const onStop = useCallback(async (name: string, jobId?: string) => {
    setBusy(b => ({ ...b, [name]: true }));
    try { await stopBackup(conn, name, jobId); await refresh(); }
    catch (e: any) { setError(e?.message || String(e)); }
    finally { setBusy(b => ({ ...b, [name]: false })); }
  }, [conn, refresh]);

  const onSaveConn = useCallback((c: Connection) => {
    saveConnection(c);
    setConn(c);
    setConnOpen(false);
  }, []);

  const docsBase = (conn.baseUrl || '').replace(/\/+$/, '');
  return html`
    <header>
      <h1>CloudBackup</h1>
      <div class="conn-info">
        Connected to <strong>${conn.baseUrl}</strong>${conn.username ? html` as <strong>${conn.username}</strong>` : null}
      </div>
      <nav class="nav-links">
        <a href=${`${docsBase}/docs/`} target="_blank" rel="noopener">Documentation</a>
        <a href=${`${docsBase}/docs_api/`} target="_blank" rel="noopener">API (Swagger)</a>
      </nav>
      <div>
        <button onClick=${() => setConnOpen(true)}>Connection</button>
        <button onClick=${() => { void refresh(); }}>Refresh</button>
      </div>
    </header>
    <main>
      ${version ? html`
        <div class="server-version">
          Server: <strong>CloudBackup ${version.CloudBackup || '?'}</strong>
          ${version.OS || version.Arch ? html` · ${version.OS || ''}${version.Arch ? '/' + version.Arch : ''}` : null}
          ${version.Runtime ? html` · Go ${version.Runtime}` : null}
          ${version.BuildDate ? html` · built ${version.BuildDate}` : null}
        </div>
      ` : null}
      ${error ? html`<div class="error">${error}</div>` : null}
      ${jobs.length === 0 && !loading && !error ? html`
        <div class="empty">No backup jobs found. Configure jobs in the server config file.</div>
      ` : null}
      <div class="jobs">
        ${jobs.map(j => html`
          <${JobCard}
            key=${j.name}
            job=${j}
            busy=${!!busy[j.name]}
            onStart=${() => onStart(j.name)}
            onStop=${() => onStop(j.name, j.job_id)}
            onWatch=${() => setWatchTarget(j)}
            onReports=${() => setReportsFor(j)}
          />
        `)}
      </div>
    </main>
    ${connOpen ? html`<${ConnectionModal} conn=${conn} onClose=${() => setConnOpen(false)} onSave=${onSaveConn} />` : null}
    ${watchTarget ? html`<${WatchModal} conn=${conn} job=${watchTarget} onClose=${() => setWatchTarget(null)} />` : null}
    ${reportsFor ? html`<${ReportsModal} conn=${conn} job=${reportsFor} onClose=${() => setReportsFor(null)} />` : null}
  `;
}

function JobCard(props: {
  job: BackupJobStatus;
  busy: boolean;
  onStart: () => void;
  onStop: () => void;
  onWatch: () => void;
  onReports: () => void;
}) {
  const { job, busy } = props;
  const running = isRunning(job.state);
  const s = job.stats_counters || {};
  const examined = (s.examined_files || 0) + (s.examined_directories || 0) + (s.examined_symlinks || 0);
  const uploaded = (s.uploaded_files || 0) + (s.uploaded_directories || 0) + (s.uploaded_symlinks || 0);
  return html`
    <div class="job">
      <header>
        <h2 title=${job.name}>${job.name}</h2>
        <span class=${'state ' + job.state}>${job.state}</span>
      </header>
      <div class="meta">
        <span><strong>Started:</strong> ${fmtTime(job.start_time)}</span>
        <span><strong>Ended:</strong> ${fmtTime(job.end_time)}</span>
        <span><strong>Next run:</strong> ${fmtTime(job.next_run)}</span>
        ${job.job_id ? html`<span><strong>Job id:</strong> <code>${job.job_id}</code></span>` : null}
      </div>
      ${running && job.stats_counters ? html`
        <div class="stats">
          <div><span>Examined</span><strong>${examined}</strong></div>
          <div><span>Uploaded</span><strong>${uploaded}</strong></div>
          <div><span>Read</span><strong>${fmtBytes(job.file_content_bytes_read)}</strong></div>
          <div><span>Rate 1m</span><strong>${fmtBytes(job.rate_1min)}/s</strong></div>
          <div><span>Failed</span><strong>${s.failed_to_upload_files || 0}</strong></div>
          <div><span>Excluded</span><strong>${s.excluded || 0}</strong></div>
        </div>
      ` : null}
      ${running && job.stats_text?.current_file ? html`
        <div class="meta"><span class="muted" title=${job.stats_text.current_file}>
          ${job.stats_text.current_file}
        </span></div>
      ` : null}
      <div class="actions">
        ${running ? html`
          <button class="danger" disabled=${busy} onClick=${props.onStop}>Stop</button>
          <button onClick=${props.onWatch}>Watch progress</button>
        ` : html`
          <button class="primary" disabled=${busy} onClick=${props.onStart}>Start now</button>
        `}
        <button onClick=${props.onReports}>Reports</button>
      </div>
    </div>
  `;
}

function ConnectionModal(props: { conn: Connection; onClose: () => void; onSave: (c: Connection) => void }) {
  const [baseUrl, setBaseUrl] = useState(props.conn.baseUrl);
  const [username, setUsername] = useState(props.conn.username);
  const [password, setPassword] = useState(props.conn.password);

  const onSave = (e: Event) => {
    e.preventDefault();
    props.onSave({ baseUrl: baseUrl.trim(), username, password });
  };

  return html`
    <div class="modal-backdrop" onClick=${(e: Event) => { if (e.target === e.currentTarget) props.onClose(); }}>
      <div class="modal" role="dialog" aria-modal="true">
        <header>
          <h2>Server connection</h2>
          <button onClick=${props.onClose}>Close</button>
        </header>
        <form onSubmit=${onSave}>
          <div class="body">
            <div class="form-row">
              <label>Base URL</label>
              <input type="url" value=${baseUrl} onInput=${(e: any) => setBaseUrl(e.currentTarget.value)}
                     placeholder="http://127.0.0.1:8080" required />
            </div>
            <div class="form-row">
              <label>Username (optional)</label>
              <input type="text" value=${username} onInput=${(e: any) => setUsername(e.currentTarget.value)}
                     autocomplete="username" />
            </div>
            <div class="form-row">
              <label>Password (optional)</label>
              <input type="password" value=${password} onInput=${(e: any) => setPassword(e.currentTarget.value)}
                     autocomplete="current-password" />
            </div>
            <p class="muted" style="font-size:12px;margin:0">
              Credentials are stored in your browser's localStorage. When connecting to a remote
              server that isn't same-origin, the remote server must allow CORS from this origin.
            </p>
          </div>
          <div class="footer">
            <button type="button" onClick=${props.onClose}>Cancel</button>
            <button type="submit" class="primary">Save</button>
          </div>
        </form>
      </div>
    </div>
  `;
}

function WatchModal(props: { conn: Connection; job: BackupJobStatus; onClose: () => void }) {
  const { conn, job } = props;
  const [events, setEvents] = useState<WatchEvent[]>([]);
  const [status, setStatus] = useState<string>('connecting…');
  const [err, setErr] = useState<string | null>(null);
  const listRef = useRef<HTMLDivElement | null>(null);
  const stickRef = useRef<boolean>(true);

  useEffect(() => {
    if (!job.job_id) { setStatus('no job id — job is not running'); return; }
    setStatus('streaming');
    const cancel = watchBackup(
      conn,
      job.name,
      job.job_id,
      (e) => {
        setEvents(prev => {
          // Collapse successive events for the same item into a single
          // line: the server emits one event per upload progress step
          // for a given file and all of those share the same sequence
          // number, so we replace the last entry in place rather than
          // appending a new line.
          const last = prev.length ? prev[prev.length - 1] : undefined;
          if (last && last.sequence === e.sequence) {
            const next = prev.slice();
            next[next.length - 1] = e;
            return next;
          }
          const next = prev.length >= MAX_EVENTS ? prev.slice(prev.length - MAX_EVENTS + 1) : prev.slice();
          next.push(e);
          return next;
        });
      },
      (reason) => setStatus(reason),
      (error) => { setErr(error.message); setStatus('error'); },
    );
    return () => cancel();
  }, [conn, job.name, job.job_id]);

  useEffect(() => {
    if (stickRef.current && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [events]);

  const onScroll = (e: Event) => {
    const el = e.currentTarget as HTMLDivElement;
    stickRef.current = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
  };

  return html`
    <div class="modal-backdrop" onClick=${(e: Event) => { if (e.target === e.currentTarget) props.onClose(); }}>
      <div class="modal" role="dialog" aria-modal="true">
        <header>
          <h2>Watching: ${job.name}</h2>
          <button onClick=${props.onClose}>Close</button>
        </header>
        <div class="body">
          <p class="muted" style="margin:0 0 8px 0">
            Status: <strong>${status}</strong>
            ${' '}· Events: <strong>${events.length}</strong>
            ${' '}· Job id: <code>${job.job_id || '-'}</code>
          </p>
          ${err ? html`<div class="error">${err}</div>` : null}
          <div class="events" ref=${listRef} onScroll=${onScroll}>
            ${events.length === 0 ? html`<div class="muted">waiting for events…</div>` : null}
            ${events.map(e => html`
              <div class="evt">
                <span class=${'op ' + e.operation_type}>${e.operation_type}</span>
                <span class="pct">${e.type === 'file' ? `${e.percent_done}%` : e.type}</span>
                <span class="name" title=${e.name}>${e.name}</span>
                ${e.error ? html`<span class="err">${e.error}</span>` : null}
              </div>
            `)}
          </div>
        </div>
        <div class="footer">
          <button onClick=${() => setEvents([])}>Clear</button>
          <button onClick=${props.onClose}>Close</button>
        </div>
      </div>
    </div>
  `;
}

function ReportsModal(props: { conn: Connection; job: BackupJobStatus; onClose: () => void }) {
  const { conn, job } = props;
  const [items, setItems] = useState<ReportListItem[]>([]);
  const [nextToken, setNextToken] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [detail, setDetail] = useState<BackupJobStatus | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const loadPage = useCallback(async (token?: string) => {
    setLoading(true);
    try {
      const r = await listReports(conn, job.name, token);
      setItems(prev => token ? prev.concat(r.items) : r.items);
      setNextToken(r.next);
      setError(null);
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setLoading(false);
    }
  }, [conn, job.name]);

  useEffect(() => { void loadPage(); }, [loadPage]);

  const openDetail = useCallback(async (jobId: string) => {
    setDetailLoading(true);
    setError(null);
    try {
      const d = await showReport(conn, job.name, jobId);
      setDetail(d);
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setDetailLoading(false);
    }
  }, [conn, job.name]);

  const backToList = () => { setDetail(null); };

  return html`
    <div class="modal-backdrop" onClick=${(e: Event) => { if (e.target === e.currentTarget) props.onClose(); }}>
      <div class="modal modal-wide" role="dialog" aria-modal="true">
        <header>
          <h2>
            ${detail ? html`Report: ${job.name}` : html`Reports: ${job.name}`}
          </h2>
          <div style="display:flex;gap:8px">
            ${detail ? html`<button onClick=${backToList}>← Back to list</button>` : null}
            <button onClick=${props.onClose}>Close</button>
          </div>
        </header>
        <div class="body">
          ${error ? html`<div class="error">${error}</div>` : null}
          ${detail
            ? html`<${ReportDetail} detail=${detail} loading=${detailLoading} />`
            : html`<${ReportList}
                items=${items}
                loading=${loading}
                nextToken=${nextToken}
                onOpen=${openDetail}
                onLoadMore=${() => loadPage(nextToken)} />`}
        </div>
      </div>
    </div>
  `;
}

function ReportList(props: {
  items: ReportListItem[];
  loading: boolean;
  nextToken: string;
  onOpen: (jobId: string) => void;
  onLoadMore: () => void;
}) {
  const { items, loading, nextToken } = props;
  return html`
    ${items.length === 0 && !loading ? html`
      <div class="empty" style="padding:24px">No reports found for this backup job.</div>
    ` : null}
    ${items.length > 0 ? html`
      <table class="report-table">
        <thead>
          <tr>
            <th>Job Id</th>
            <th>State</th>
            <th>Duration</th>
            <th>Start time</th>
            <th>End time</th>
          </tr>
        </thead>
        <tbody>
          ${items.map(it => html`
            <tr class="clickable" onClick=${() => props.onOpen(it.job_id)}>
              <td><code>${it.job_id}</code></td>
              <td><span class=${'state ' + it.state}>${it.state}</span></td>
              <td>${fmtDuration(it.start_time, it.end_time)}</td>
              <td>${fmtTime(it.start_time)}</td>
              <td>${fmtTime(it.end_time)}</td>
            </tr>
          `)}
        </tbody>
      </table>
    ` : null}
    <div class="report-actions">
      ${loading ? html`<span class="muted"><span class="spinner"></span> Loading…</span>` : null}
      ${nextToken && !loading ? html`<button onClick=${props.onLoadMore}>Load more</button>` : null}
    </div>
  `;
}

function ReportDetail(props: { detail: BackupJobStatus; loading: boolean }) {
  const d = props.detail;
  const s = d.stats_counters || {};
  const g = (k: string): number => {
    const v = s[k];
    return typeof v === 'number' ? v : 0;
  };
  const sections: Array<{ title: string; rows: Array<[string, any]> }> = [
    {
      title: 'Overview',
      rows: [
        ['Name', d.name],
        ['State', html`<span class=${'state ' + d.state}>${d.state}</span>`],
        ['Job id', html`<code>${d.job_id || '-'}</code>`],
        ['Platform', d.platform || '-'],
        ['Start time', fmtTime(d.start_time)],
        ['End time', fmtTime(d.end_time)],
        ['Duration', fmtDuration(d.start_time, d.end_time)],
        ['Next run', fmtTime(d.next_run)],
      ],
    },
    {
      title: 'Transfer',
      rows: [
        ['File content read', fmtBytes(d.file_content_bytes_read)],
        ['Rate (1 min)', `${fmtBytes(d.rate_1min)}/s`],
        ['Rate (5 min)', `${fmtBytes(d.rate_5min)}/s`],
        ['Rate (15 min)', `${fmtBytes(d.rate_15min)}/s`],
      ],
    },
    {
      title: 'Examination',
      rows: [
        ['Directories examined', g('examined_directories')],
        ['Files examined', g('examined_files')],
        ['Symlinks examined', g('examined_symlinks')],
        ['Unordinary files examined', g('examined_unknown')],
        ['Excluded by rule', g('excluded')],
        ['Failed to examine', g('failed_to_examine')],
        ['Failed to enumerate directory', g('failed_to_enumerate')],
      ],
    },
    {
      title: 'Uploads',
      rows: [
        ['Files uploaded', g('uploaded_files')],
        ['Directories uploaded', g('uploaded_directories')],
        ['Symlinks uploaded', g('uploaded_symlinks')],
        ['Files up to date', g('up_to_date_files')],
        ['Directories up to date', g('up_to_date_directories')],
        ['Symlinks up to date', g('up_to_date_symlinks')],
        ['Files failed to upload', g('failed_to_upload_files')],
        ['Directories failed to upload', g('failed_to_upload_directories')],
        ['Symlinks failed to upload', g('failed_to_upload_symlinks')],
        ['Unordinary files failed to upload', g('failed_to_upload_unknown')],
      ],
    },
    {
      title: 'Metadata-only updates',
      rows: [
        ['Files updated', g('updated_metadata_for_files')],
        ['Directories updated', g('updated_metadata_for_directories')],
        ['Symlinks updated', g('updated_metadata_for_symlinks')],
        ['Files failed', g('failed_to_update_metadata_for_files')],
        ['Directories failed', g('failed_to_update_metadata_for_directories')],
        ['Symlinks failed', g('failed_to_update_metadata_for_symlinks')],
      ],
    },
    {
      title: 'Deletion tracking',
      rows: [
        ['Files marked deleted', g('marked_deleted_files')],
        ['Directories marked deleted', g('marked_deleted_directories')],
        ['Symlinks marked deleted', g('marked_deleted_symlinks')],
        ['Failed to mark files deleted', g('failed_to_mark_deleted_files')],
        ['Failed to mark dirs deleted', g('failed_to_mark_deleted_directories')],
        ['Failed to mark symlinks deleted', g('failed_to_mark_deleted_symlinks')],
        ['Failed to build deleted list', g('failed_to_find_deleted')],
      ],
    },
    {
      title: 'Scripts & database',
      rows: [
        ['Scripts defined', g('scripts_num')],
        ['Scripts ran', g('scripts_ran')],
        ['Scripts failed', g('scripts_failed')],
        ['Database copy errors', g('database_copy_errors')],
      ],
    },
  ];
  const currentDir = d.stats_text?.current_directory || '';
  const currentFile = d.stats_text?.current_file || '';
  const dirLabel = isRunning(d.state) ? 'Current directory' : 'Last processed directory';
  const fileLabel = isRunning(d.state) ? 'Current file' : 'Last processed file';
  return html`
    ${props.loading ? html`<div class="muted"><span class="spinner"></span> Loading…</div>` : null}
    ${currentDir || currentFile ? html`
      <div class="server-version" style="margin-bottom:14px">
        ${currentDir ? html`<div><strong>${dirLabel}:</strong> <code>${currentDir}</code></div>` : null}
        ${currentFile ? html`<div><strong>${fileLabel}:</strong> <code>${currentFile}</code></div>` : null}
      </div>
    ` : null}
    <div class="report-detail">
      ${sections.map(sec => html`
        <div class="report-section">
          <h3>${sec.title}</h3>
          <div class="kv">
            ${sec.rows.map(([k, v]) => html`
              <div class="kv-row"><span class="kv-key">${k}</span><span class="kv-val">${v}</span></div>
            `)}
          </div>
        </div>
      `)}
    </div>
  `;
}

export function mount(el: HTMLElement): void {
  render(html`<${App} />`, el);
}
