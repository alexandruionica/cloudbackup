function authHeader(c) {
    if (!c.username && !c.password)
        return {};
    return { Authorization: 'Basic ' + btoa(`${c.username}:${c.password}`) };
}
function url(c, path) {
    const base = (c.baseUrl || '').replace(/\/+$/, '');
    return `${base}/api/v1${path}`;
}
async function jsonRequest(c, path, method, body) {
    const res = await fetch(url(c, path), {
        method,
        headers: {
            'Content-Type': 'application/json',
            ...authHeader(c),
        },
        body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    const text = await res.text();
    let json = {};
    if (text) {
        try {
            json = JSON.parse(text);
        }
        catch {
            throw new Error(`Invalid JSON (HTTP ${res.status}): ${text.slice(0, 200)}`);
        }
    }
    if (!res.ok || (json && json.code === 'error')) {
        throw new Error(json?.message || `HTTP ${res.status}`);
    }
    return json;
}
export async function listBackups(c) {
    const r = await jsonRequest(c, '/backup/list', 'GET');
    return r.result ?? [];
}
export async function startBackup(c, name) {
    const r = await jsonRequest(c, '/backup/start', 'POST', { name });
    if (!r.result)
        throw new Error('Server returned no result');
    return r.result;
}
export async function stopBackup(c, name, jobId) {
    const body = { name };
    if (jobId)
        body.job_id = jobId;
    await jsonRequest(c, '/backup/stop', 'POST', body);
}
export async function listReports(c, name, next) {
    const body = { name };
    if (next)
        body.next = next;
    const r = await jsonRequest(c, '/report/backup/list', 'POST', body);
    return { items: r.result ?? [], next: r.next ?? '' };
}
export async function showReport(c, name, jobId) {
    const r = await jsonRequest(c, '/report/backup/show', 'POST', { name, job_id: jobId });
    if (!r.result)
        throw new Error('Server returned no result');
    return r.result;
}
export async function getVersion(c) {
    // The server returns result as a single object despite what the
    // swagger doc declares (httpd/misc_handlers.go:handlerVersion passes
    // the struct directly to JSONSuccessWithResult, not wrapped in an
    // array).
    const r = await jsonRequest(c, '/report/version', 'GET');
    return r.result ?? null;
}
export async function listRestores(c) {
    const r = await jsonRequest(c, '/restore/list', 'GET');
    return r.result ?? [];
}
export async function startRestore(c, input) {
    const r = await jsonRequest(c, '/restore/start', 'POST', input);
    if (!r.result)
        throw new Error('Server returned no result');
    return r.result;
}
export async function stopRestore(c, name, restoreJobId) {
    const body = { name };
    if (restoreJobId)
        body.restore_job_id = restoreJobId;
    await jsonRequest(c, '/restore/stop', 'POST', body);
}
export async function resumeRestore(c, name, targetName, restoreJobId) {
    const r = await jsonRequest(c, '/restore/resume', 'POST', {
        name, target_name: targetName, restore_job_id: restoreJobId,
    });
    if (!r.result)
        throw new Error('Server returned no result');
    return r.result;
}
export async function listRestoreReports(c, name, next) {
    const body = { name };
    if (next)
        body.next = next;
    const r = await jsonRequest(c, '/report/restore/list', 'POST', body);
    return { items: r.result ?? [], next: r.next ?? '' };
}
export async function showRestoreReport(c, name, jobId) {
    const r = await jsonRequest(c, '/report/restore/show', 'POST', { name, job_id: jobId });
    if (!r.result)
        throw new Error('Server returned no result');
    return r.result;
}
export async function listBackupFiles(c, name, jobId, path, descend, next) {
    const body = { name, job_id: jobId, path, descend };
    if (next)
        body.next = next;
    const r = await jsonRequest(c, '/report/backup/file/list', 'POST', body);
    return { items: r.result ?? [], next: r.next ?? '' };
}
export async function getConfig(c) {
    const r = await jsonRequest(c, '/config', 'GET');
    return r.result ?? null;
}
/**
 * Stream live watch events from one of the SSE endpoints. The cloudbackup
 * server emits "data: <json>\n" lines (single LF, not the standard "\n\n"
 * SSE terminator). Trailing terminator messages start with "Backup job ",
 * "Restore job " or "Completed run".
 */
function streamSseEvents(c, path, body, onEvent, onClose, onError) {
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
            const handleLine = (rawLine) => {
                const line = rawLine.replace(/\r$/, '');
                if (!line.startsWith('data:'))
                    return false;
                const data = line.replace(/^data:\s?/, '');
                if (!data)
                    return false;
                if (data.startsWith('Backup job ') || data.startsWith('Restore job ') || data.startsWith('Completed run')) {
                    onClose(data);
                    return true;
                }
                try {
                    onEvent(JSON.parse(data));
                }
                catch { /* ignore non-JSON payloads */ }
                return false;
            };
            while (true) {
                const { done, value } = await reader.read();
                if (done) {
                    if (buf && handleLine(buf))
                        return;
                    onClose('stream ended');
                    return;
                }
                buf += decoder.decode(value, { stream: true });
                let nl;
                while ((nl = buf.indexOf('\n')) !== -1) {
                    const line = buf.slice(0, nl);
                    buf = buf.slice(nl + 1);
                    if (handleLine(line))
                        return;
                }
            }
        }
        catch (e) {
            if (e?.name === 'AbortError')
                return;
            onError(e instanceof Error ? e : new Error(String(e)));
        }
    })();
    return () => ctrl.abort();
}
export function watchBackup(c, name, jobId, onEvent, onClose, onError) {
    return streamSseEvents(c, '/backup/watch', { name, job_id: jobId }, onEvent, onClose, onError);
}
export function watchRestore(c, name, restoreJobId, onEvent, onClose, onError) {
    return streamSseEvents(c, '/restore/watch', { name, restore_job_id: restoreJobId }, onEvent, onClose, onError);
}
