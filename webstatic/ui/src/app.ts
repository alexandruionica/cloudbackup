import { html } from 'htm/preact';
import { render } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  getConfig, getVersion, listBackupFiles, listBackups, listReports, listRestoreReports,
  listRestores, resumeRestore, showReport, showRestoreReport, startBackup, startRestore,
  stopBackup, stopRestore, watchBackup, watchRestore,
  type BackupJobStatus, type ConfigBackup, type ConfigBackupTarget, type Connection,
  type FileListEntry, type ReportListItem, type ServerVersion, type WatchEvent,
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

// "stopped" and "crashed" restore reports can be resumed (see restore.go:Resume).
function isResumable(state: string): boolean {
  return state === 'stopped' || state === 'crashed';
}

interface WatchTarget {
  kind: 'backup' | 'restore';
  name: string;
  jobId: string;
  title: string;
}

function App() {
  const [conn, setConn] = useState<Connection>(loadConnection);
  const [jobs, setJobs] = useState<BackupJobStatus[]>([]);
  const [restoresByName, setRestoresByName] = useState<Record<string, BackupJobStatus>>({});
  const [version, setVersion] = useState<ServerVersion | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [connOpen, setConnOpen] = useState(false);
  const [watchTarget, setWatchTarget] = useState<WatchTarget | null>(null);
  const [reportsFor, setReportsFor] = useState<BackupJobStatus | null>(null);
  const [restoreReportsFor, setRestoreReportsFor] = useState<BackupJobStatus | null>(null);
  const [restoreStartFor, setRestoreStartFor] = useState<BackupJobStatus | null>(null);
  const [busy, setBusy] = useState<Record<string, boolean>>({});

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [list, restores] = await Promise.all([listBackups(conn), listRestores(conn)]);
      setJobs(list);
      const map: Record<string, BackupJobStatus> = {};
      for (const r of restores) map[r.name] = r;
      setRestoresByName(map);
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

  const onStopRestore = useCallback(async (name: string, restoreJobId?: string) => {
    const key = name + '::restore';
    setBusy(b => ({ ...b, [key]: true }));
    try { await stopRestore(conn, name, restoreJobId); await refresh(); }
    catch (e: any) { setError(e?.message || String(e)); }
    finally { setBusy(b => ({ ...b, [key]: false })); }
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
            restore=${restoresByName[j.name]}
            busy=${!!busy[j.name]}
            restoreBusy=${!!busy[j.name + '::restore']}
            onStart=${() => onStart(j.name)}
            onStop=${() => onStop(j.name, j.job_id)}
            onWatch=${() => setWatchTarget({ kind: 'backup', name: j.name, jobId: j.job_id || '', title: j.name })}
            onReports=${() => setReportsFor(j)}
            onRestore=${() => setRestoreStartFor(j)}
            onWatchRestore=${(rid: string) => setWatchTarget({ kind: 'restore', name: j.name, jobId: rid, title: j.name + ' (restore)' })}
            onStopRestore=${(rid: string) => onStopRestore(j.name, rid)}
            onRestoreReports=${() => setRestoreReportsFor(j)}
          />
        `)}
      </div>
    </main>
    ${connOpen ? html`<${ConnectionModal} conn=${conn} onClose=${() => setConnOpen(false)} onSave=${onSaveConn} />` : null}
    ${watchTarget ? html`<${WatchModal} conn=${conn} target=${watchTarget} onClose=${() => setWatchTarget(null)} />` : null}
    ${reportsFor ? html`<${ReportsModal} conn=${conn} job=${reportsFor} onClose=${() => setReportsFor(null)} />` : null}
    ${restoreReportsFor ? html`<${RestoreReportsModal}
        conn=${conn}
        job=${restoreReportsFor}
        restoreInProgress=${!!restoresByName[restoreReportsFor.name]}
        onClose=${() => setRestoreReportsFor(null)}
        onResumed=${() => { setRestoreReportsFor(null); void refresh(); }} />` : null}
    ${restoreStartFor ? html`<${RestoreStartModal}
        conn=${conn}
        job=${restoreStartFor}
        onClose=${() => setRestoreStartFor(null)}
        onStarted=${() => { setRestoreStartFor(null); void refresh(); }} />` : null}
  `;
}

function JobCard(props: {
  job: BackupJobStatus;
  restore?: BackupJobStatus;
  busy: boolean;
  restoreBusy: boolean;
  onStart: () => void;
  onStop: () => void;
  onWatch: () => void;
  onReports: () => void;
  onRestore: () => void;
  onWatchRestore: (jobId: string) => void;
  onStopRestore: (jobId: string) => void;
  onRestoreReports: () => void;
}) {
  const { job, busy, restore, restoreBusy } = props;
  const running = isRunning(job.state);
  const restoreRunning = !!restore && isRunning(restore.state);
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
          <button class="primary" disabled=${busy || restoreRunning} onClick=${props.onStart}>Start now</button>
        `}
        <button onClick=${props.onReports}>Reports</button>
      </div>
      <div class="restore-row">
        <div class="restore-row-head">
          <span class="restore-label">Restore</span>
          ${restoreRunning ? html`<span class=${'state ' + restore!.state}>${restore!.state}</span>` : html`<span class="state">idle</span>`}
        </div>
        ${restoreRunning ? html`
          <div class="meta">
            ${restore!.job_id ? html`<span><strong>Restore id:</strong> <code>${restore!.job_id}</code></span>` : null}
            <span><strong>Started:</strong> ${fmtTime(restore!.start_time)}</span>
            ${restore!.stats_text?.current_file ? html`<span class="muted" title=${restore!.stats_text.current_file}>${restore!.stats_text.current_file}</span>` : null}
          </div>
        ` : null}
        <div class="actions">
          ${restoreRunning ? html`
            <button class="danger" disabled=${restoreBusy} onClick=${() => props.onStopRestore(restore!.job_id || '')}>Stop restore</button>
            <button onClick=${() => props.onWatchRestore(restore!.job_id || '')}>Watch restore</button>
          ` : html`
            <button class="primary" disabled=${running} onClick=${props.onRestore}>Restore…</button>
          `}
          <button onClick=${props.onRestoreReports}>Restore reports</button>
        </div>
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

function WatchModal(props: { conn: Connection; target: WatchTarget; onClose: () => void }) {
  const { conn, target } = props;
  const [events, setEvents] = useState<WatchEvent[]>([]);
  const [status, setStatus] = useState<string>('connecting…');
  const [err, setErr] = useState<string | null>(null);
  // "follow" drives both UI (the "Jump to latest" button visibility) and
  // auto-scroll behavior. Mirror it into a ref so the scroll/effect callbacks
  // always read the current value without stale-closure issues.
  const [follow, setFollow] = useState<boolean>(true);
  const followRef = useRef<boolean>(true);
  const listRef = useRef<HTMLDivElement | null>(null);
  // Flag set immediately before a programmatic scrollTop write so the resulting
  // scroll event is ignored (otherwise a late-firing event from our own write
  // could race against a newly-arrived event and incorrectly disable follow).
  const programmaticScroll = useRef<boolean>(false);

  useEffect(() => { followRef.current = follow; }, [follow]);

  useEffect(() => {
    if (!target.jobId) { setStatus('no job id — job is not running'); return; }
    setStatus('streaming');
    const handler = target.kind === 'backup' ? watchBackup : watchRestore;
    const cancel = handler(
      conn,
      target.name,
      target.jobId,
      (e) => {
        setEvents(prev => {
          // Collapse successive events for the same item (same sequence)
          // into a single line — the server emits one event per upload
          // progress step for a given file, all sharing the same sequence
          // number.
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
  }, [conn, target.kind, target.name, target.jobId]);

  useEffect(() => {
    const el = listRef.current;
    if (!el) return;
    if (followRef.current) {
      programmaticScroll.current = true;
      el.scrollTop = el.scrollHeight;
    }
  }, [events]);

  const onScroll = (e: Event) => {
    if (programmaticScroll.current) {
      programmaticScroll.current = false;
      return;
    }
    const el = e.currentTarget as HTMLDivElement;
    const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
    if (atBottom !== followRef.current) setFollow(atBottom);
  };

  const jumpToEnd = () => {
    const el = listRef.current;
    if (!el) return;
    programmaticScroll.current = true;
    el.scrollTop = el.scrollHeight;
    setFollow(true);
  };

  return html`
    <div class="modal-backdrop" onClick=${(e: Event) => { if (e.target === e.currentTarget) props.onClose(); }}>
      <div class="modal" role="dialog" aria-modal="true">
        <header>
          <h2>Watching: ${target.title}</h2>
          <button onClick=${props.onClose}>Close</button>
        </header>
        <div class="body">
          <p class="muted" style="margin:0 0 8px 0">
            Status: <strong>${status}</strong>
            ${' '}· Events: <strong>${events.length}</strong>
            ${' '}· Job id: <code>${target.jobId || '-'}</code>
            ${' '}· Follow: <strong>${follow ? 'on' : 'off'}</strong>
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
          ${!follow ? html`<button onClick=${jumpToEnd}>Jump to latest</button>` : null}
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

// ---------------------------------------------------------------------------
// Restore: report list / detail + resume
// ---------------------------------------------------------------------------

function RestoreReportsModal(props: {
  conn: Connection;
  job: BackupJobStatus;
  restoreInProgress: boolean;
  onClose: () => void;
  onResumed: () => void;
}) {
  const { conn, job } = props;
  const [items, setItems] = useState<ReportListItem[]>([]);
  const [nextToken, setNextToken] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [detail, setDetail] = useState<BackupJobStatus | null>(null);
  const [detailJobId, setDetailJobId] = useState<string>('');
  const [detailState, setDetailState] = useState<string>('');
  const [detailLoading, setDetailLoading] = useState(false);
  const [resuming, setResuming] = useState(false);
  const [targets, setTargets] = useState<ConfigBackupTarget[]>([]);
  const [pickedTarget, setPickedTarget] = useState<string>('');

  const loadPage = useCallback(async (token?: string) => {
    setLoading(true);
    try {
      const r = await listRestoreReports(conn, job.name, token);
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

  // Targets from /config — needed because the resume API requires
  // target_name and the report-list rows do not carry it.
  useEffect(() => {
    let stopped = false;
    (async () => {
      try {
        const cfg = await getConfig(conn);
        if (stopped) return;
        const b = (cfg?.backup ?? []).find(x => x.name === job.name);
        const t = b?.target ?? [];
        setTargets(t);
        if (t.length === 1) setPickedTarget(t[0].name);
      } catch {
        // non-fatal: resume just becomes unavailable
      }
    })();
    return () => { stopped = true; };
  }, [conn, job.name]);

  const openDetail = useCallback(async (jobId: string, state: string) => {
    setDetailLoading(true);
    setDetailJobId(jobId);
    setDetailState(state);
    setError(null);
    try {
      const d = await showRestoreReport(conn, job.name, jobId);
      setDetail(d);
    } catch (e: any) {
      // 'crashed' restores may have no stored report — surface message but keep the resume option.
      setDetail(null);
      setError(e?.message || String(e));
    } finally {
      setDetailLoading(false);
    }
  }, [conn, job.name]);

  const backToList = () => {
    setDetail(null);
    setDetailJobId('');
    setDetailState('');
    setError(null);
  };

  const onResume = useCallback(async (jobId: string) => {
    if (!pickedTarget) {
      setError('Please pick a target to resume from');
      return;
    }
    setResuming(true);
    setError(null);
    try {
      await resumeRestore(conn, job.name, pickedTarget, jobId);
      props.onResumed();
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setResuming(false);
    }
  }, [conn, job.name, pickedTarget, props]);

  const showingDetail = !!detail || !!detailJobId;
  const canResumeCurrent = showingDetail && isResumable(detailState);
  return html`
    <div class="modal-backdrop" onClick=${(e: Event) => { if (e.target === e.currentTarget) props.onClose(); }}>
      <div class="modal modal-wide" role="dialog" aria-modal="true">
        <header>
          <h2>${showingDetail ? html`Restore report: ${job.name}` : html`Restore reports: ${job.name}`}</h2>
          <div style="display:flex;gap:8px">
            ${showingDetail ? html`<button onClick=${backToList}>← Back to list</button>` : null}
            <button onClick=${props.onClose}>Close</button>
          </div>
        </header>
        <div class="body">
          ${error ? html`<div class="error">${error}</div>` : null}
          ${canResumeCurrent ? html`
            <div class="resume-bar">
              <span class="muted">This restore is in <strong>${detailState}</strong> state and can be resumed.</span>
              ${targets.length > 1 ? html`
                <label class="muted" style="text-transform:none;letter-spacing:0">Target:
                  <select value=${pickedTarget} onChange=${(e: any) => setPickedTarget(e.currentTarget.value)}>
                    <option value="">(pick one)</option>
                    ${targets.map(t => html`<option value=${t.name}>${t.name}${t.type ? ' — ' + t.type : ''}</option>`)}
                  </select>
                </label>
              ` : null}
              <button class="primary" disabled=${resuming || props.restoreInProgress || !pickedTarget}
                      onClick=${() => onResume(detailJobId)}>
                ${resuming ? 'Resuming…' : 'Resume restore'}
              </button>
              ${props.restoreInProgress ? html`<span class="muted">A restore for this job is already running.</span>` : null}
            </div>
          ` : null}
          ${showingDetail
            ? (detail ? html`<${ReportDetail} detail=${detail} loading=${detailLoading} />` : html`
                <div class="muted">${detailLoading ? html`<span class="spinner"></span> Loading…` : 'No report payload available.'}</div>
              `)
            : html`<${RestoreReportList}
                items=${items}
                loading=${loading}
                nextToken=${nextToken}
                resumable=${(s: string) => isResumable(s)}
                onOpen=${(jobId: string, state: string) => openDetail(jobId, state)}
                onLoadMore=${() => loadPage(nextToken)} />`}
        </div>
      </div>
    </div>
  `;
}

function RestoreReportList(props: {
  items: ReportListItem[];
  loading: boolean;
  nextToken: string;
  resumable: (state: string) => boolean;
  onOpen: (jobId: string, state: string) => void;
  onLoadMore: () => void;
}) {
  const { items, loading, nextToken } = props;
  return html`
    ${items.length === 0 && !loading ? html`
      <div class="empty" style="padding:24px">No restore reports found for this backup job.</div>
    ` : null}
    ${items.length > 0 ? html`
      <table class="report-table">
        <thead>
          <tr>
            <th>Restore Job Id</th>
            <th>State</th>
            <th>Duration</th>
            <th>Start time</th>
            <th>End time</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          ${items.map(it => html`
            <tr class="clickable" onClick=${() => props.onOpen(it.job_id, it.state)}>
              <td><code>${it.job_id}</code></td>
              <td><span class=${'state ' + it.state}>${it.state}</span></td>
              <td>${fmtDuration(it.start_time, it.end_time)}</td>
              <td>${fmtTime(it.start_time)}</td>
              <td>${fmtTime(it.end_time)}</td>
              <td>${props.resumable(it.state) ? html`<span class="resumable-tag">resumable</span>` : null}</td>
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

// ---------------------------------------------------------------------------
// Restore start: pick source backup -> pick files (or all) -> options -> submit
// ---------------------------------------------------------------------------

interface FileTreeItem {
  path: string;
  parent: string;
  isDir: boolean;
  size: number;
}

function fileEntryToItem(e: FileListEntry): FileTreeItem | null {
  if (!e.instances || e.instances.length === 0) return null;
  const inst = e.instances[0];
  const t = (inst.type || '').toLowerCase();
  return {
    path: e.path,
    parent: e.parent,
    isDir: t === 'directory' || t === 'dir',
    size: inst.size || 0,
  };
}

function RestoreStartModal(props: {
  conn: Connection;
  job: BackupJobStatus;
  onClose: () => void;
  onStarted: () => void;
}) {
  const { conn, job } = props;
  // Step 1: source backup selection.
  const [sourceJobs, setSourceJobs] = useState<ReportListItem[]>([]);
  const [sourceLoading, setSourceLoading] = useState(false);
  const [sourceJobId, setSourceJobId] = useState<string>('');
  const [error, setError] = useState<string | null>(null);

  // Step 2: file selection.
  const [allFiles, setAllFiles] = useState<boolean>(false);
  const [cwd, setCwd] = useState<string>(''); // '' == virtual root
  const [stack, setStack] = useState<string[]>([]);
  const [entries, setEntries] = useState<FileTreeItem[]>([]);
  const [browsing, setBrowsing] = useState(false);
  const [browseErr, setBrowseErr] = useState<string | null>(null);
  const [selected, setSelected] = useState<Record<string, true>>({});

  // Step 3: options.
  const [targets, setTargets] = useState<ConfigBackupTarget[]>([]);
  const [targetName, setTargetName] = useState<string>('');
  const [restoreDir, setRestoreDir] = useState<string>('');
  const [exclusionsText, setExclusionsText] = useState<string>('');
  const [submitting, setSubmitting] = useState(false);

  // Load list of past finished/stopped backup jobs to pick a source.
  useEffect(() => {
    let stopped = false;
    setSourceLoading(true);
    (async () => {
      try {
        const r = await listReports(conn, job.name);
        if (stopped) return;
        setSourceJobs(r.items);
        // Auto-pick the most recent finished one when present.
        const finished = r.items.find(x => x.state === 'finished');
        const candidate = finished || r.items[0];
        if (candidate) setSourceJobId(candidate.job_id);
      } catch (e: any) {
        if (!stopped) setError(e?.message || String(e));
      } finally {
        if (!stopped) setSourceLoading(false);
      }
    })();
    return () => { stopped = true; };
  }, [conn, job.name]);

  // Targets for the dropdown.
  useEffect(() => {
    let stopped = false;
    (async () => {
      try {
        const cfg = await getConfig(conn);
        if (stopped) return;
        const b: ConfigBackup | undefined = (cfg?.backup ?? []).find(x => x.name === job.name);
        setTargets(b?.target ?? []);
      } catch { /* non-fatal */ }
    })();
    return () => { stopped = true; };
  }, [conn, job.name]);

  // Browse helper. cwd == '' means virtual root: ask server with descend=true,
  // then client-side filter to top-level entries (parent not in returned set).
  const loadDir = useCallback(async (path: string) => {
    if (!sourceJobId) return;
    setBrowsing(true);
    setBrowseErr(null);
    try {
      const descend = path === '';
      const r = await listBackupFiles(conn, job.name, sourceJobId, path, descend);
      const items: FileTreeItem[] = [];
      const pathSet = new Set<string>();
      for (const e of r.items) pathSet.add(e.path);
      for (const e of r.items) {
        if (e.path === path && path !== '') continue;
        const item = fileEntryToItem(e);
        if (!item) continue;
        if (descend) {
          // Only surface backup-root entries: their parent is not also in the result set.
          if (pathSet.has(item.parent)) continue;
        }
        items.push(item);
      }
      items.sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        return a.path.localeCompare(b.path);
      });
      setEntries(items);
    } catch (e: any) {
      setBrowseErr(e?.message || String(e));
    } finally {
      setBrowsing(false);
    }
  }, [conn, job.name, sourceJobId]);

  // When source job changes (or all_files toggled off), reload from root.
  useEffect(() => {
    if (allFiles) return;
    setEntries([]);
    setStack([]);
    setCwd('');
    if (sourceJobId) void loadDir('');
  }, [sourceJobId, allFiles, loadDir]);

  const enterDir = useCallback((path: string) => {
    setStack(s => [...s, cwd]);
    setCwd(path);
    void loadDir(path);
  }, [cwd, loadDir]);

  const goUp = useCallback(() => {
    if (stack.length === 0) return;
    const parent = stack[stack.length - 1];
    setStack(s => s.slice(0, -1));
    setCwd(parent);
    void loadDir(parent);
  }, [stack, loadDir]);

  const toggleSelect = useCallback((path: string) => {
    setSelected(prev => {
      const next = { ...prev };
      if (next[path]) delete next[path]; else next[path] = true;
      return next;
    });
  }, []);

  const selectedPaths = useMemo(() => Object.keys(selected).sort(), [selected]);

  const onSubmit = useCallback(async () => {
    setError(null);
    if (!sourceJobId) { setError('Please pick a source backup job to restore from.'); return; }
    if (!allFiles && selectedPaths.length === 0) {
      setError('Pick at least one file or directory, or check "Restore all files".');
      return;
    }
    setSubmitting(true);
    try {
      const exclusions = exclusionsText.split('\n').map(s => s.trim()).filter(Boolean);
      await startRestore(conn, {
        name: job.name,
        source_backup_job_id: sourceJobId,
        target_name: targetName || undefined,
        restore_dir: restoreDir || undefined,
        files: allFiles ? undefined : selectedPaths,
        all_files: allFiles || undefined,
        exclusions: exclusions.length ? exclusions : undefined,
      });
      props.onStarted();
    } catch (e: any) {
      setError(e?.message || String(e));
    } finally {
      setSubmitting(false);
    }
  }, [conn, job.name, sourceJobId, targetName, restoreDir, exclusionsText, allFiles, selectedPaths, props]);

  return html`
    <div class="modal-backdrop" onClick=${(e: Event) => { if (e.target === e.currentTarget) props.onClose(); }}>
      <div class="modal modal-wide" role="dialog" aria-modal="true">
        <header>
          <h2>Restore from: ${job.name}</h2>
          <button onClick=${props.onClose}>Close</button>
        </header>
        <div class="body">
          ${error ? html`<div class="error">${error}</div>` : null}
          <div class="form-row">
            <label>Source backup job (the run from which to fetch files)</label>
            ${sourceLoading ? html`<span class="muted"><span class="spinner"></span> Loading available backup runs…</span>` : null}
            <select value=${sourceJobId} onChange=${(e: any) => setSourceJobId(e.currentTarget.value)} disabled=${sourceLoading}>
              <option value="">— select —</option>
              ${sourceJobs.map(it => html`
                <option value=${it.job_id}>${fmtTime(it.start_time)} · ${it.state} · ${it.job_id.slice(0, 8)}…</option>
              `)}
            </select>
          </div>

          <div class="form-row">
            <label>What to restore</label>
            <label class="muted" style="text-transform:none;letter-spacing:0;display:flex;align-items:center;gap:6px">
              <input type="checkbox" style="min-width:auto;width:auto" checked=${allFiles}
                     onChange=${(e: any) => setAllFiles(e.currentTarget.checked)} />
              Restore <strong>all files</strong> recorded for this backup run
            </label>
          </div>

          ${!allFiles ? html`
            <div class="form-row">
              <label>Pick files and/or directories
                <span class="muted" style="text-transform:none;letter-spacing:0">(${selectedPaths.length} selected)</span>
              </label>
              <div class="browser">
                <div class="browser-bar">
                  <button onClick=${goUp} disabled=${stack.length === 0 || browsing}>↑ Up</button>
                  <span class="browser-cwd" title=${cwd || '(roots)'}><code>${cwd || '(backup roots)'}</code></span>
                  <button onClick=${() => loadDir(cwd)} disabled=${browsing || !sourceJobId}>Refresh</button>
                </div>
                ${browseErr ? html`<div class="error" style="margin:8px 0">${browseErr}</div>` : null}
                <div class="browser-list">
                  ${browsing && entries.length === 0 ? html`<div class="muted"><span class="spinner"></span> Loading…</div>` : null}
                  ${!browsing && entries.length === 0 && !browseErr ? html`<div class="muted">empty</div>` : null}
                  ${entries.map(e => html`
                    <div class="browser-row">
                      <input type="checkbox" checked=${!!selected[e.path]}
                             onChange=${() => toggleSelect(e.path)}
                             title=${'Select ' + e.path} />
                      ${e.isDir ? html`
                        <button class="browser-name dir" onClick=${() => enterDir(e.path)} title=${'Open ' + e.path}>
                          <span class="icon">📁</span><span class="path">${e.path}</span>
                        </button>
                      ` : html`
                        <span class="browser-name">
                          <span class="icon">📄</span><span class="path" title=${e.path}>${e.path}</span>
                          <span class="muted size">${fmtBytes(e.size)}</span>
                        </span>
                      `}
                    </div>
                  `)}
                </div>
                ${selectedPaths.length > 0 ? html`
                  <details class="browser-selected">
                    <summary>${selectedPaths.length} selected (click to view)</summary>
                    <ul>${selectedPaths.map(p => html`
                      <li><code>${p}</code> <button class="link-btn" onClick=${() => toggleSelect(p)}>remove</button></li>
                    `)}</ul>
                  </details>
                ` : null}
              </div>
            </div>
          ` : null}

          <div class="form-row">
            <label>Target (object store) — optional, defaults to first</label>
            <select value=${targetName} onChange=${(e: any) => setTargetName(e.currentTarget.value)}>
              <option value="">(default: first defined target)</option>
              ${targets.map(t => html`<option value=${t.name}>${t.name}${t.type ? ' — ' + t.type : ''}</option>`)}
            </select>
          </div>

          <div class="form-row">
            <label>Restore directory override (server-side path, optional)</label>
            <input type="text" value=${restoreDir} onInput=${(e: any) => setRestoreDir(e.currentTarget.value)}
                   placeholder="leave empty to use the server's configured restore_dir" />
          </div>

          <div class="form-row">
            <label>Exclusion patterns — optional, one per line</label>
            <textarea rows="3" class="ta" value=${exclusionsText}
                      onInput=${(e: any) => setExclusionsText(e.currentTarget.value)}
                      placeholder="e.g. **/*.log&#10;/data/cache/**"></textarea>
          </div>
        </div>
        <div class="footer">
          <button onClick=${props.onClose} disabled=${submitting}>Cancel</button>
          <button class="primary" onClick=${onSubmit} disabled=${submitting || !sourceJobId}>
            ${submitting ? 'Starting…' : 'Start restore'}
          </button>
        </div>
      </div>
    </div>
  `;
}

export function mount(el: HTMLElement): void {
  render(html`<${App} />`, el);
}
