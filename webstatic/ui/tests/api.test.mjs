// Unit tests for the compiled api.js module. Run with Node >= 18:
//   cd webstatic/ui && node --test tests/api.test.mjs
//
// These tests stub globalThis.fetch to avoid any real network traffic and
// feed synthetic Response objects (including streamed SSE responses) into
// the exported API functions.

import test from 'node:test';
import assert from 'node:assert/strict';
import {
  listBackups,
  startBackup,
  stopBackup,
  getVersion,
  listReports,
  showReport,
  watchBackup,
} from '../js/api.js';

const conn = { baseUrl: 'http://example:8080', username: 'u', password: 'p' };
const anon = { baseUrl: 'http://example:8080', username: '', password: '' };

function stubFetch(handler) {
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    return handler(url, init);
  };
  return calls;
}

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function streamResponse(chunks, status = 200) {
  const enc = new TextEncoder();
  const body = new ReadableStream({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c));
      controller.close();
    },
  });
  return new Response(body, {
    status,
    headers: { 'content-type': 'text/event-stream' },
  });
}

function runWatch(chunks, { ...extras } = {}) {
  stubFetch(() => streamResponse(chunks));
  const events = [];
  let closeReason = null;
  let err = null;
  return new Promise((resolve) => {
    const cancel = watchBackup(
      extras.conn ?? anon,
      extras.name ?? 'j',
      extras.jobId ?? 'uuid',
      (e) => events.push(e),
      (r) => { closeReason = r; resolve({ events, closeReason, err, cancel }); },
      (e) => { err = e; resolve({ events, closeReason, err, cancel }); },
    );
  });
}

test.afterEach(() => {
  delete globalThis.fetch;
});

// ---------------------------------------------------------------------------
// listBackups
// ---------------------------------------------------------------------------

test('listBackups: GET /api/v1/backup/list with basic auth header', async () => {
  const calls = stubFetch(() => jsonResponse({
    code: 'success', message: 'ok', result: [{ name: 'j', state: 'stopped' }],
  }));
  const jobs = await listBackups(conn);
  assert.equal(calls[0].url, 'http://example:8080/api/v1/backup/list');
  assert.equal(calls[0].init.method, 'GET');
  assert.equal(
    calls[0].init.headers.Authorization,
    'Basic ' + Buffer.from('u:p').toString('base64'),
  );
  assert.deepEqual(jobs, [{ name: 'j', state: 'stopped' }]);
});

test('listBackups: no Authorization header when credentials are empty', async () => {
  const calls = stubFetch(() => jsonResponse({ code: 'success', message: 'ok', result: [] }));
  await listBackups(anon);
  assert.equal(calls[0].init.headers.Authorization, undefined);
});

test('listBackups: strips trailing slashes from baseUrl', async () => {
  const calls = stubFetch(() => jsonResponse({ code: 'success', message: 'ok', result: [] }));
  await listBackups({ ...anon, baseUrl: 'http://example:8080///' });
  assert.equal(calls[0].url, 'http://example:8080/api/v1/backup/list');
});

test('listBackups: returns empty array when server omits result', async () => {
  stubFetch(() => jsonResponse({ code: 'success', message: 'ok' }));
  const jobs = await listBackups(anon);
  assert.deepEqual(jobs, []);
});

test('listBackups: throws when payload code === "error"', async () => {
  stubFetch(() => jsonResponse({ code: 'error', message: 'nope' }));
  await assert.rejects(() => listBackups(anon), /nope/);
});

test('listBackups: throws on HTTP non-2xx with server-provided message', async () => {
  stubFetch(() => jsonResponse({ code: 'error', message: 'not authorized' }, 401));
  await assert.rejects(() => listBackups(anon), /not authorized/);
});

test('listBackups: throws on invalid JSON payload', async () => {
  stubFetch(() => new Response('not json at all', {
    status: 200,
    headers: { 'content-type': 'text/plain' },
  }));
  await assert.rejects(() => listBackups(anon), /Invalid JSON/);
});

// ---------------------------------------------------------------------------
// startBackup / stopBackup
// ---------------------------------------------------------------------------

test('startBackup: POSTs { name } and returns the result object', async () => {
  const calls = stubFetch(() => jsonResponse({
    code: 'success', message: 'ok', result: { name: 'j', job_id: 'uuid-1' },
  }));
  const r = await startBackup(anon, 'j');
  assert.equal(calls[0].url, 'http://example:8080/api/v1/backup/start');
  assert.equal(calls[0].init.method, 'POST');
  assert.equal(calls[0].init.headers['Content-Type'], 'application/json');
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: 'j' });
  assert.deepEqual(r, { name: 'j', job_id: 'uuid-1' });
});

test('startBackup: throws when server does not return a result', async () => {
  stubFetch(() => jsonResponse({ code: 'success', message: 'ok' }));
  await assert.rejects(() => startBackup(anon, 'j'), /Server returned no result/);
});

test('stopBackup: body omits job_id when none is given', async () => {
  const calls = stubFetch(() => jsonResponse({ code: 'success', message: 'ok' }));
  await stopBackup(anon, 'j');
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: 'j' });
});

test('stopBackup: body includes job_id when given', async () => {
  const calls = stubFetch(() => jsonResponse({ code: 'success', message: 'ok' }));
  await stopBackup(anon, 'j', 'uuid-1');
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: 'j', job_id: 'uuid-1' });
});

// ---------------------------------------------------------------------------
// getVersion (regression — the server returns result as a single object,
// not an array as the swagger document claims).
// ---------------------------------------------------------------------------

test('getVersion: returns result as a single object (not array[0])', async () => {
  stubFetch(() => jsonResponse({
    code: 'success',
    message: 'ok',
    result: { CloudBackup: 'v1.2.3', OS: 'linux', Arch: 'amd64' },
  }));
  const v = await getVersion(anon);
  assert.equal(v.CloudBackup, 'v1.2.3');
  assert.equal(v.OS, 'linux');
  assert.equal(v.Arch, 'amd64');
});

test('getVersion: returns null when result is missing', async () => {
  stubFetch(() => jsonResponse({ code: 'success', message: 'ok' }));
  const v = await getVersion(anon);
  assert.equal(v, null);
});

// ---------------------------------------------------------------------------
// listReports / showReport
// ---------------------------------------------------------------------------

test('listReports: POSTs { name } and returns items + next token', async () => {
  const calls = stubFetch(() => jsonResponse({
    code: 'success',
    message: 'ok',
    next: 'TOKEN=',
    result: [
      { name: 'j', job_id: 'uid-1', start_time: '2026-04-11T10:00:00Z', end_time: '2026-04-11T10:05:00Z', state: 'finished' },
    ],
  }));
  const page = await listReports(anon, 'j');
  assert.equal(calls[0].url, 'http://example:8080/api/v1/report/backup/list');
  assert.equal(calls[0].init.method, 'POST');
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: 'j' });
  assert.equal(page.next, 'TOKEN=');
  assert.equal(page.items.length, 1);
  assert.equal(page.items[0].job_id, 'uid-1');
});

test('listReports: forwards next token when paging', async () => {
  const calls = stubFetch(() => jsonResponse({ code: 'success', message: 'ok', result: [] }));
  await listReports(anon, 'j', 'PAGE2=');
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: 'j', next: 'PAGE2=' });
});

test('listReports: returns empty list / empty next when server omits both', async () => {
  stubFetch(() => jsonResponse({ code: 'success', message: 'ok' }));
  const page = await listReports(anon, 'j');
  assert.deepEqual(page.items, []);
  assert.equal(page.next, '');
});

test('showReport: POSTs { name, job_id } and returns the job status', async () => {
  const status = {
    name: 'j', state: 'finished', job_id: 'uid-1',
    start_time: '2026-04-11T10:00:00Z', end_time: '2026-04-11T10:05:00Z',
    stats_counters: { uploaded_files: 42 },
  };
  const calls = stubFetch(() => jsonResponse({ code: 'success', message: 'ok', result: status }));
  const r = await showReport(anon, 'j', 'uid-1');
  assert.equal(calls[0].url, 'http://example:8080/api/v1/report/backup/show');
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: 'j', job_id: 'uid-1' });
  assert.deepEqual(r, status);
});

test('showReport: throws when server returns no result', async () => {
  stubFetch(() => jsonResponse({ code: 'success', message: 'ok' }));
  await assert.rejects(() => showReport(anon, 'j', 'uid-1'), /Server returned no result/);
});

// ---------------------------------------------------------------------------
// watchBackup — SSE parser. The cloudbackup server emits one event per
// "data: <json>\n" line (single LF, not the standard "\n\n" terminator),
// so the parser must be line-oriented. See api_rest_backup.go:560.
// ---------------------------------------------------------------------------

const sampleEvent = (seq, pct) => ({
  sequence: seq,
  name: '/etc/hosts',
  percent_done: pct,
  rate: 100,
  type: 'file',
  store_name: 's',
  store_type: 'aws_s3',
  operation_type: 'upload',
  error: '',
});

test('watchBackup: parses one event per line terminated by \\n', async () => {
  const e1 = sampleEvent(1, 50);
  const e2 = sampleEvent(1, 100);
  const { events, closeReason, err } = await runWatch([
    `data: ${JSON.stringify(e1)}\n`,
    `data: ${JSON.stringify(e2)}\n`,
    `data: Backup job has finished\n`,
  ]);
  assert.equal(err, null);
  assert.deepEqual(events, [e1, e2]);
  assert.equal(closeReason, 'Backup job has finished');
});

test('watchBackup: reassembles events split across multiple read() chunks', async () => {
  const ev = sampleEvent(5, 100);
  const json = JSON.stringify(ev);
  const { events, closeReason, err } = await runWatch([
    'data: ' + json.slice(0, 10),
    json.slice(10) + '\ndata: Backup job has finished\n',
  ]);
  assert.equal(err, null);
  assert.deepEqual(events, [ev]);
  assert.equal(closeReason, 'Backup job has finished');
});

test('watchBackup: accepts CRLF line endings', async () => {
  const ev = sampleEvent(1, 50);
  const { events, closeReason, err } = await runWatch([
    `data: ${JSON.stringify(ev)}\r\ndata: Backup job has finished\r\n`,
  ]);
  assert.equal(err, null);
  assert.deepEqual(events, [ev]);
  assert.equal(closeReason, 'Backup job has finished');
});

test('watchBackup: reports "Backup job was cancelled" as close reason', async () => {
  const { events, closeReason } = await runWatch([
    'data: Backup job was cancelled while it was running\n',
  ]);
  assert.deepEqual(events, []);
  assert.equal(closeReason, 'Backup job was cancelled while it was running');
});

test('watchBackup: reports "Completed run" (dryrun terminator) as close reason', async () => {
  const { closeReason } = await runWatch(['data: Completed run\n']);
  assert.equal(closeReason, 'Completed run');
});

test('watchBackup: silently ignores non-JSON data lines', async () => {
  const ev = sampleEvent(1, 1);
  const { events, closeReason } = await runWatch([
    'data: not a json line\n',
    `data: ${JSON.stringify(ev)}\n`,
    'data: Backup job has finished\n',
  ]);
  assert.deepEqual(events, [ev]);
  assert.equal(closeReason, 'Backup job has finished');
});

test('watchBackup: HTTP non-2xx surfaces as onError, not onClose', async () => {
  stubFetch(() => new Response('backup not running', { status: 400 }));
  let err = null;
  let closeCalled = false;
  await new Promise((resolve) => {
    watchBackup(
      anon, 'j', 'u',
      () => {},
      () => { closeCalled = true; resolve(); },
      (e) => { err = e; resolve(); },
    );
  });
  assert.ok(err);
  assert.match(err.message, /HTTP 400/);
  assert.equal(closeCalled, false);
});

test('watchBackup: POST body and headers match the API contract', async () => {
  const calls = stubFetch(() => streamResponse(['data: Backup job has finished\n']));
  await new Promise((resolve) => {
    watchBackup(
      anon, 'myjob', 'uuid-123',
      () => {},
      () => resolve(),
      () => resolve(),
    );
  });
  assert.equal(calls[0].url, 'http://example:8080/api/v1/backup/watch');
  assert.equal(calls[0].init.method, 'POST');
  assert.deepEqual(JSON.parse(calls[0].init.body), { name: 'myjob', job_id: 'uuid-123' });
  assert.equal(calls[0].init.headers['Content-Type'], 'application/json');
  assert.equal(calls[0].init.headers['Accept'], 'text/event-stream');
});

test('watchBackup: cancel() aborts the request and suppresses AbortError', async () => {
  let aborted = false;
  stubFetch((_url, init) => new Promise((_, reject) => {
    init.signal.addEventListener('abort', () => {
      aborted = true;
      const err = new Error('aborted');
      err.name = 'AbortError';
      reject(err);
    });
  }));
  let errCalled = false;
  let closeCalled = false;
  const cancel = watchBackup(
    anon, 'j', 'u',
    () => {},
    () => { closeCalled = true; },
    () => { errCalled = true; },
  );
  // give the async fetch() microtask a chance to register the abort listener
  await new Promise((r) => setTimeout(r, 10));
  cancel();
  await new Promise((r) => setTimeout(r, 10));
  assert.equal(aborted, true, 'fetch should see abort signal');
  assert.equal(errCalled, false, 'AbortError must not surface as onError');
  assert.equal(closeCalled, false, 'AbortError must not surface as onClose');
});
