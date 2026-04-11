import { html } from 'htm/preact';
import { render } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import { getVersion, listBackups, startBackup, stopBackup, watchBackup, } from './api.js';
const STORAGE_KEY = 'cloudbackup.connection';
const POLL_MS = 3000;
const MAX_EVENTS = 500;
function defaultConnection() {
    const base = location.protocol === 'file:'
        ? 'http://127.0.0.1:8080'
        : `${location.protocol}//${location.host}`;
    return { baseUrl: base, username: '', password: '' };
}
function loadConnection() {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (raw) {
            const parsed = JSON.parse(raw);
            if (parsed && typeof parsed.baseUrl === 'string')
                return parsed;
        }
    }
    catch { /* ignore */ }
    return defaultConnection();
}
function saveConnection(c) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(c));
    }
    catch { /* ignore */ }
}
function fmtBytes(n) {
    if (n === undefined || n === null || isNaN(n))
        return '-';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let v = n, u = 0;
    while (v >= 1024 && u < units.length - 1) {
        v /= 1024;
        u++;
    }
    return `${u === 0 ? v.toFixed(0) : v.toFixed(1)} ${units[u]}`;
}
function fmtTime(s) {
    if (!s || s.startsWith('0001-01-01'))
        return '-';
    const d = new Date(s);
    if (isNaN(d.getTime()))
        return s;
    return d.toLocaleString();
}
function isRunning(state) {
    return state === 'running' || state === 'started' || state === 'stopping';
}
function App() {
    const [conn, setConn] = useState(loadConnection);
    const [jobs, setJobs] = useState([]);
    const [version, setVersion] = useState(null);
    const [error, setError] = useState(null);
    const [loading, setLoading] = useState(false);
    const [connOpen, setConnOpen] = useState(false);
    const [watchTarget, setWatchTarget] = useState(null);
    const [busy, setBusy] = useState({});
    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const list = await listBackups(conn);
            setJobs(list);
            setError(null);
        }
        catch (e) {
            setError(e?.message || String(e));
        }
        finally {
            setLoading(false);
        }
    }, [conn]);
    useEffect(() => {
        let stopped = false;
        const tick = async () => {
            if (stopped)
                return;
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
                if (!stopped)
                    setVersion(v);
            }
            catch {
                // version is a best-effort display; surface any auth/network
                // trouble through the main error channel from listBackups.
            }
        })();
        return () => { stopped = true; };
    }, [conn]);
    const onStart = useCallback(async (name) => {
        setBusy(b => ({ ...b, [name]: true }));
        try {
            await startBackup(conn, name);
            await refresh();
        }
        catch (e) {
            setError(e?.message || String(e));
        }
        finally {
            setBusy(b => ({ ...b, [name]: false }));
        }
    }, [conn, refresh]);
    const onStop = useCallback(async (name, jobId) => {
        setBusy(b => ({ ...b, [name]: true }));
        try {
            await stopBackup(conn, name, jobId);
            await refresh();
        }
        catch (e) {
            setError(e?.message || String(e));
        }
        finally {
            setBusy(b => ({ ...b, [name]: false }));
        }
    }, [conn, refresh]);
    const onSaveConn = useCallback((c) => {
        saveConnection(c);
        setConn(c);
        setConnOpen(false);
    }, []);
    return html `
    <header>
      <h1>CloudBackup</h1>
      <div class="conn-info">
        Connected to <strong>${conn.baseUrl}</strong>${conn.username ? html ` as <strong>${conn.username}</strong>` : null}
      </div>
      <div>
        <button onClick=${() => setConnOpen(true)}>Connection</button>
        <button onClick=${() => { void refresh(); }} disabled=${loading}>
          ${loading ? html `<span class="spinner"></span>` : 'Refresh'}
        </button>
      </div>
    </header>
    <main>
      ${version ? html `
        <div class="server-version">
          Server: <strong>CloudBackup ${version.CloudBackup || '?'}</strong>
          ${version.OS || version.Arch ? html ` · ${version.OS || ''}${version.Arch ? '/' + version.Arch : ''}` : null}
          ${version.Runtime ? html ` · Go ${version.Runtime}` : null}
          ${version.BuildDate ? html ` · built ${version.BuildDate}` : null}
        </div>
      ` : null}
      ${error ? html `<div class="error">${error}</div>` : null}
      ${jobs.length === 0 && !loading && !error ? html `
        <div class="empty">No backup jobs found. Configure jobs in the server config file.</div>
      ` : null}
      <div class="jobs">
        ${jobs.map(j => html `
          <${JobCard}
            key=${j.name}
            job=${j}
            busy=${!!busy[j.name]}
            onStart=${() => onStart(j.name)}
            onStop=${() => onStop(j.name, j.job_id)}
            onWatch=${() => setWatchTarget(j)}
          />
        `)}
      </div>
    </main>
    ${connOpen ? html `<${ConnectionModal} conn=${conn} onClose=${() => setConnOpen(false)} onSave=${onSaveConn} />` : null}
    ${watchTarget ? html `<${WatchModal} conn=${conn} job=${watchTarget} onClose=${() => setWatchTarget(null)} />` : null}
  `;
}
function JobCard(props) {
    const { job, busy } = props;
    const running = isRunning(job.state);
    const s = job.stats_counters || {};
    const examined = (s.examined_files || 0) + (s.examined_directories || 0) + (s.examined_symlinks || 0);
    const uploaded = (s.uploaded_files || 0) + (s.uploaded_directories || 0) + (s.uploaded_symlinks || 0);
    return html `
    <div class="job">
      <header>
        <h2 title=${job.name}>${job.name}</h2>
        <span class=${'state ' + job.state}>${job.state}</span>
      </header>
      <div class="meta">
        <span><strong>Started:</strong> ${fmtTime(job.start_time)}</span>
        <span><strong>Ended:</strong> ${fmtTime(job.end_time)}</span>
        <span><strong>Next run:</strong> ${fmtTime(job.next_run)}</span>
        ${job.job_id ? html `<span><strong>Job id:</strong> <code>${job.job_id}</code></span>` : null}
      </div>
      ${running && job.stats_counters ? html `
        <div class="stats">
          <div><span>Examined</span><strong>${examined}</strong></div>
          <div><span>Uploaded</span><strong>${uploaded}</strong></div>
          <div><span>Read</span><strong>${fmtBytes(job.file_content_bytes_read)}</strong></div>
          <div><span>Rate 1m</span><strong>${fmtBytes(job.rate_1min)}/s</strong></div>
          <div><span>Failed</span><strong>${s.failed_to_upload_files || 0}</strong></div>
          <div><span>Excluded</span><strong>${s.excluded || 0}</strong></div>
        </div>
      ` : null}
      ${running && job.stats_text?.current_file ? html `
        <div class="meta"><span class="muted" title=${job.stats_text.current_file}>
          ${job.stats_text.current_file}
        </span></div>
      ` : null}
      <div class="actions">
        ${running ? html `
          <button class="danger" disabled=${busy} onClick=${props.onStop}>Stop</button>
          <button onClick=${props.onWatch}>Watch progress</button>
        ` : html `
          <button class="primary" disabled=${busy} onClick=${props.onStart}>Start now</button>
        `}
      </div>
    </div>
  `;
}
function ConnectionModal(props) {
    const [baseUrl, setBaseUrl] = useState(props.conn.baseUrl);
    const [username, setUsername] = useState(props.conn.username);
    const [password, setPassword] = useState(props.conn.password);
    const onSave = (e) => {
        e.preventDefault();
        props.onSave({ baseUrl: baseUrl.trim(), username, password });
    };
    return html `
    <div class="modal-backdrop" onClick=${(e) => { if (e.target === e.currentTarget)
        props.onClose(); }}>
      <div class="modal" role="dialog" aria-modal="true">
        <header>
          <h2>Server connection</h2>
          <button onClick=${props.onClose}>Close</button>
        </header>
        <form onSubmit=${onSave}>
          <div class="body">
            <div class="form-row">
              <label>Base URL</label>
              <input type="url" value=${baseUrl} onInput=${(e) => setBaseUrl(e.currentTarget.value)}
                     placeholder="http://127.0.0.1:8080" required />
            </div>
            <div class="form-row">
              <label>Username (optional)</label>
              <input type="text" value=${username} onInput=${(e) => setUsername(e.currentTarget.value)}
                     autocomplete="username" />
            </div>
            <div class="form-row">
              <label>Password (optional)</label>
              <input type="password" value=${password} onInput=${(e) => setPassword(e.currentTarget.value)}
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
function WatchModal(props) {
    const { conn, job } = props;
    const [events, setEvents] = useState([]);
    const [status, setStatus] = useState('connecting…');
    const [err, setErr] = useState(null);
    const listRef = useRef(null);
    const stickRef = useRef(true);
    useEffect(() => {
        if (!job.job_id) {
            setStatus('no job id — job is not running');
            return;
        }
        setStatus('streaming');
        const cancel = watchBackup(conn, job.name, job.job_id, (e) => {
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
        }, (reason) => setStatus(reason), (error) => { setErr(error.message); setStatus('error'); });
        return () => cancel();
    }, [conn, job.name, job.job_id]);
    useEffect(() => {
        if (stickRef.current && listRef.current) {
            listRef.current.scrollTop = listRef.current.scrollHeight;
        }
    }, [events]);
    const onScroll = (e) => {
        const el = e.currentTarget;
        stickRef.current = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
    };
    return html `
    <div class="modal-backdrop" onClick=${(e) => { if (e.target === e.currentTarget)
        props.onClose(); }}>
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
          ${err ? html `<div class="error">${err}</div>` : null}
          <div class="events" ref=${listRef} onScroll=${onScroll}>
            ${events.length === 0 ? html `<div class="muted">waiting for events…</div>` : null}
            ${events.map(e => html `
              <div class="evt">
                <span class=${'op ' + e.operation_type}>${e.operation_type}</span>
                <span class="pct">${e.type === 'file' ? `${e.percent_done}%` : e.type}</span>
                <span class="name" title=${e.name}>${e.name}</span>
                ${e.error ? html `<span class="err">${e.error}</span>` : null}
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
export function mount(el) {
    render(html `<${App} />`, el);
}
