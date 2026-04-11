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
/**
 * Stream live watch events for a job. Uses fetch() streaming because the native
 * EventSource cannot POST a request body nor send an Authorization header.
 * Returns a function that cancels the stream.
 */
export function watchBackup(c, name, jobId, onEvent, onClose, onError) {
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
            const handleLine = (rawLine) => {
                const line = rawLine.replace(/\r$/, '');
                if (!line.startsWith('data:'))
                    return false;
                const data = line.replace(/^data:\s?/, '');
                if (!data)
                    return false;
                if (data.startsWith('Backup job ') || data.startsWith('Completed run')) {
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
