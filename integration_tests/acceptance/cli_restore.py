#!/usr/bin/env python
"""
User acceptance tests for "cloudbackup client restore ..." (user guide sections 2.8 and 6.3).

Every test runs a real backup of a temp tree through a persistent test_null target, then drives the
restore through the shipped CLI binary exactly as the guide documents it, and verifies the bytes that
land on disk. The interactive TUI file browser is covered by Go unit tests (client/restore/browse_test.go);
here every invocation passes --non-interactive or an explicit selection.
"""
import os
import re
import shlex
import time

import pytest

from common import *


RESTORE_ID_RE = re.compile(r"Restore job id '([0-9a-f-]{36})' has been allocated")
JOB = "first_backup"


def restore_id_from_output(result):
    m = RESTORE_ID_RE.search(result.stdout)
    assert m, "could not find the allocated restore job id in: {}".format(result)
    return m.group(1)


def q(path):
    return shlex.quote(path)


# ---- 6.3 "Start a restore": everything from that run ---------------------------------------------------

def test_restore_all_files_restores_every_byte(api, client_config, source_tree, restore_dir):
    """`client restore start <job> -i <id> --all-files --restore-dir <dir>` then `restore report show`."""
    backup_id = api.run_backup_and_wait(JOB)

    r = run_cli("client restore start {} -i {} --all-files --restore-dir {} -N".format(JOB, backup_id, q(restore_dir)),
                client_config)
    restore_id = restore_id_from_output(r)
    api.wait_restore_finishes(JOB, restore_id)

    num_files, num_dirs, num_symlinks = verify_restored_tree(api, restore_dir, source_tree.root, source_tree.filelist)
    check_restore_report(api, JOB, restore_id, num_files, num_dirs, num_symlinks)

    # the same report through the CLI, as JSON
    shown = run_cli("client restore report show -i {} {} --json".format(restore_id, JOB), client_config).json()
    assert shown['result']['job_id'] == restore_id
    assert shown['result']['state'] == 'finished'
    assert shown['result']['stats_counters']['restored_files'] == num_files
    assert shown['result']['stats_counters']['failed_to_restore_files'] == 0

    # and the human-readable variant mentions the job and the outcome
    text = run_cli("client restore report show -i {} {}".format(restore_id, JOB), client_config).stdout
    assert restore_id in text and "finished" in text


def test_restore_start_json_output_is_the_server_response(api, client_config, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    r = run_cli("client restore start {} -i {} --all-files --restore-dir {} -N --json".format(
        JOB, backup_id, q(restore_dir)), client_config)
    decoded = r.json()
    assert decoded['code'] == 'success'
    assert decoded['result']['name'] == JOB
    restore_id = decoded['result']['restore_job_id']
    api.wait_restore_finishes(JOB, restore_id)


def test_restore_start_with_watch_streams_until_finished(api, client_config, source_tree, restore_dir):
    """`client restore start ... --watch` blocks until the restore is done and prints the closing line."""
    backup_id = api.run_backup_and_wait(JOB)
    api.set_target_ratelimit(JOB, "500")
    r = run_cli("client restore start {} -i {} --all-files --restore-dir {} -N --watch".format(
        JOB, backup_id, q(restore_dir)), client_config)
    assert "Restore job has finished" in r.stdout, r
    assert "Percent" in r.stdout and "OpType" in r.stdout, "expected the progress table header in: {}".format(r)
    verify_restored_tree(api, restore_dir, source_tree.root, source_tree.filelist)


# ---- 6.3 "Start a restore": explicit files and exclusions ---------------------------------------------

def test_restore_selected_file_and_directory(api, client_config, source_tree, restore_dir):
    """Repeated --file: one regular file plus one directory (with trailing separator) restores exactly those."""
    backup_id = api.run_backup_and_wait(JOB)
    a_dir = os.path.join(source_tree.root, "dir1", "dir2") + os.sep
    a_file = os.path.join(source_tree.root, "dir1", ";=&'file9.txt")
    r = run_cli("client restore start {} -i {} --restore-dir {} --file {} --file {}".format(
        JOB, backup_id, q(restore_dir), q(a_dir), q(a_file)), client_config)
    restore_id = restore_id_from_output(r)
    api.wait_restore_finishes(JOB, restore_id)
    verify_restored_subset(api, restore_dir, source_tree.filelist, [a_dir, a_file])
    report = api.restore_report(JOB, restore_id)
    assert report['state'] == 'finished'
    assert report['stats_counters']['failed_to_restore_files'] == 0


def test_restore_with_exclusion_pattern(api, client_config, source_tree, restore_dir):
    """--exclusion takes a glob; '**/*.htm*' keeps every HTML file out of an --all-files restore."""
    backup_id = api.run_backup_and_wait(JOB)
    r = run_cli("client restore start {} -i {} --all-files --restore-dir {} --exclusion '**/*.htm*'".format(
        JOB, backup_id, q(restore_dir)), client_config)
    restore_id = restore_id_from_output(r)
    api.wait_restore_finishes(JOB, restore_id)
    html = [p for p, t in source_tree.filelist.items() if t == "file" and ".htm" in os.path.basename(p)]
    rest = {p: t for p, t in source_tree.filelist.items() if p not in html}
    assert html, "fixture tree is expected to contain HTML files"
    for p in html:
        assert not os.path.exists(map_path_into_restore_dir(restore_dir, p)), "excluded file was restored: {}".format(p)
    for p, t in rest.items():
        restored = map_path_into_restore_dir(restore_dir, p)
        assert os.path.exists(restored), "non-excluded item missing after restore: {}".format(p)
        if t == "file":
            assert get_md5_sum(p) == get_md5_sum(restored)
    report = api.restore_report(JOB, restore_id)
    assert report['stats_counters']['restored_files'] == len([t for t in rest.values() if t == "file"])


def test_restore_files_and_all_files_are_mutually_exclusive(api, client_config, source_tree, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    a_file = os.path.join(source_tree.root, "dir1", "dir2", "file1.txt")
    r = run_cli("client restore start {} -i {} --all-files --file {} --restore-dir {}".format(
        JOB, backup_id, q(a_file), q(restore_dir)), client_config, expect=1)
    assert "mutually exclusive" in r.stdout.lower() or "mutually exclusive" in r.stderr.lower(), r


def test_non_interactive_without_selection_is_rejected(api, client_config, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    r = run_cli("client restore start {} -i {} --restore-dir {} --non-interactive".format(
        JOB, backup_id, q(restore_dir)), client_config, expect=1)
    assert "Neither --all-files nor --file were supplied" in r.stdout, r
    assert not os.listdir(restore_dir), "nothing must be restored when the request is rejected"


def test_restore_from_unknown_backup_job_id_is_accepted_then_vanishes(api, client_config, restore_dir):
    """
    Pins the current contract, which is a usability gap worth fixing: the server accepts the request
    (the source job id is only checked once the restore goroutine runs), so the CLI exits 0 with an
    allocated id; the job then aborts before a report row is written, so `restore report show` for that
    id answers 404 and nothing lands on disk. The user gets "Successfully requested" and no trace after.
    """
    api.run_backup_and_wait(JOB)
    r = run_cli("client restore start {} -i 00000000-0000-4000-8000-000000000000 --all-files --restore-dir {} -N".format(
        JOB, q(restore_dir)), client_config)
    restore_id = restore_id_from_output(r)
    api.wait_restore_finishes(JOB, restore_id)
    resp = api.raw_post('/report/restore/show', {"name": JOB, "job_id": restore_id})
    assert resp.status_code == 404, "expected no report row for an aborted restore, got {}: {}".format(
        resp.status_code, resp.text)
    run_cli("client restore report show -i {} {}".format(restore_id, JOB), client_config, expect=1)
    assert not os.listdir(restore_dir), "nothing must be restored from an unknown source job"


# ---- 6.3 "Monitor and stop restores" -------------------------------------------------------------------

def test_restore_list_watch_and_finish(api, client_config, source_tree, restore_dir):
    """`restore list` shows the running job, `restore watch --json` streams per-item events until done."""
    backup_id = api.run_backup_and_wait(JOB)
    # the test_null store serves restores instantly; throttle the target so the job is observable
    api.set_target_ratelimit(JOB, "200")
    r = run_cli("client restore start {} -i {} --all-files --restore-dir {} -N".format(JOB, backup_id, q(restore_dir)),
                client_config)
    restore_id = restore_id_from_output(r)

    listed = run_cli("client restore list --json", client_config).json()
    running = [e for e in listed['result'] if e['name'] == JOB and e.get('job_id') == restore_id]
    assert running, "running restore {} not present in `restore list --json`: {}".format(restore_id, listed)
    assert running[0]['state'] == 'running'
    plain = run_cli("client restore list", client_config)
    assert restore_id in plain.stdout and JOB in plain.stdout, plain

    watched = run_cli("client restore watch {} -i {} --json".format(JOB, restore_id), client_config)
    docs, text = decode_json_stream(watched.stdout)
    assert "Restore job has finished" in " ".join(text), watched
    assert docs, "expected per-item restore events in the watch stream: {}".format(watched)
    assert all(d.get("operation_type") != "upload" for d in docs), "backup events leaked into a restore watch"

    api.wait_restore_finishes(JOB, restore_id)
    after = run_cli("client restore list", client_config)
    assert "No restore jobs are currently running." in after.stdout, after
    verify_restored_tree(api, restore_dir, source_tree.root, source_tree.filelist)


def test_restore_watch_plain_output(api, client_config, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    api.set_target_ratelimit(JOB, "200")
    r = run_cli("client restore start {} -i {} --all-files --restore-dir {} -N".format(JOB, backup_id, q(restore_dir)),
                client_config)
    restore_id = restore_id_from_output(r)
    watched = run_cli("client restore watch {} -i {}".format(JOB, restore_id), client_config)
    assert "Restore job has finished" in watched.stdout, watched
    assert "Percent" in watched.stdout, "expected the progress table header in: {}".format(watched)
    api.wait_restore_finishes(JOB, restore_id)


def test_restore_stop_cancels_a_running_restore(api, client_config, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    api.set_target_ratelimit(JOB, "50")
    r = run_cli("client restore start {} -i {} --all-files --restore-dir {} -N".format(JOB, backup_id, q(restore_dir)),
                client_config)
    restore_id = restore_id_from_output(r)
    assert api.restore_running(JOB, restore_id)

    stopped = run_cli("client restore stop {} -i {}".format(JOB, restore_id), client_config)
    assert stopped.stdout.strip(), stopped
    api.wait_restore_finishes(JOB, restore_id)
    report = api.restore_report(JOB, restore_id)
    assert report['state'] == 'cancelled', report

    # the report list through the CLI agrees
    listed = run_cli("client restore report list {} --json".format(JOB), client_config).json()
    mine = [e for e in listed['result'] if e['job_id'] == restore_id]
    assert mine and mine[0]['state'] == 'cancelled', listed


def test_restore_stop_when_nothing_is_running_fails(api, client_config):
    api.run_backup_and_wait(JOB)
    r = run_cli("client restore stop {}".format(JOB), client_config, expect=1)
    assert r.stdout.strip(), r


# ---- 6.3 "Restore reports" -----------------------------------------------------------------------------

def test_restore_report_list_filters_by_start_time(api, client_config, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    restore_id = api.start_restore(JOB, backup_id, restore_dir, all_files=True)
    api.wait_restore_finishes(JOB, restore_id)

    listed = run_cli("client restore report list {} --json".format(JOB), client_config).json()
    ids = [e['job_id'] for e in listed['result']]
    assert restore_id in ids, listed

    table = run_cli("client restore report list {}".format(JOB), client_config)
    assert restore_id in table.stdout, table

    # a window that closed an hour ago cannot contain the restore that just ran
    start = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() - 7200))
    until = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() - 3600))
    empty = run_cli("client restore report list {} --json --from-start-time {} --until-start-time {}".format(
        JOB, start, until), client_config).json()
    assert not empty['result'], "a window that ended an hour ago must list no restores: {}".format(empty)


def test_restore_report_show_requires_job_id(api, client_config):
    api.run_backup_and_wait(JOB)
    run_cli("client restore report show {}".format(JOB), client_config, expect=1)


# ---- access control ------------------------------------------------------------------------------------

def test_read_only_user_cannot_start_restore_but_can_list(api, client_config, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    ro = " -u testuser2 -p " + q("Oonaawai8Eep]eethe8eefa$")
    r = run_cli("client restore start {} -i {} --all-files --restore-dir {} -N{}".format(
        JOB, backup_id, q(restore_dir), ro), client_config, expect=1)
    assert "does not have access to" in r.stdout.lower(), r
    assert not os.listdir(restore_dir)
    run_cli("client restore list" + ro, client_config)
    run_cli("client restore report list {}{}".format(JOB, ro), client_config)
