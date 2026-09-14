#!/usr/bin/env python
"""
Scenario for user guide 7.4, "Resuming a failed restore": a restore that ended prematurely can be resumed
into the same directory and skips what was already restored. The web UI calls POST /restore/resume; the
CLI has no resume command, so this scenario drives the API directly.
"""
import os
import time

from common import *


JOB = "first_backup"
TARGET = "aws_1"  # first target of first_backup in the shared test config


def restored_file_count(restore_dir):
    total = 0
    for _, _, files in os.walk(restore_dir):
        total += len(files)
    return total


def test_stopped_restore_can_be_resumed_to_completion(api, source_tree, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    num_files, num_dirs, _ = count_files_folders_links(source_tree.root)

    # throttle so the restore takes several seconds, then stop it once some files have landed
    api.set_target_ratelimit(JOB, "120")
    restore_id = api.start_restore(JOB, backup_id, restore_dir, all_files=True)
    deadline = time.time() + 30
    while restored_file_count(restore_dir) == 0 and time.time() < deadline:
        time.sleep(0.1)
    assert api.restore_running(JOB, restore_id), "restore finished before it could be interrupted; lower the ratelimit"
    api.post("/restore/stop", {"name": JOB, "restore_job_id": restore_id})
    api.wait_restore_finishes(JOB, restore_id)
    partial = api.restore_report(JOB, restore_id)
    assert partial["state"] == "cancelled", partial
    done_before = partial["stats_counters"]["restored_files"]
    assert 0 < done_before < num_files, "expected a partial restore, got {} of {} files".format(done_before, num_files)

    # resume: same job id, same directory
    api.set_target_ratelimit(JOB, "0")
    resumed = api.post("/restore/resume", {"name": JOB, "target_name": TARGET, "restore_job_id": restore_id})
    assert resumed["result"]["restore_job_id"] == restore_id, resumed
    api.wait_restore_finishes(JOB, restore_id)

    verify_restored_tree(api, restore_dir, source_tree.root, source_tree.filelist)
    final = api.restore_report(JOB, restore_id)
    assert final["state"] == "finished", final
    assert final["stats_counters"]["failed_to_restore_files"] == 0
    assert final["stats_counters"]["restored_files"] == num_files, \
        "the report of a resumed job must account for every file across both attempts: {}".format(final)


def test_resume_of_a_finished_restore_is_refused_and_leaves_the_report_alone(api, restore_dir):
    """
    /restore/resume is accepted synchronously (the scheduler validates the stored state), so the API answers
    200; the refusal then shows up as a log line only, and the finished report must remain finished with
    its counters intact.
    """
    backup_id = api.run_backup_and_wait(JOB)
    restore_id = api.start_restore(JOB, backup_id, restore_dir, all_files=True)
    api.wait_restore_finishes(JOB, restore_id)
    before = api.restore_report(JOB, restore_id)
    assert before["state"] == "finished"

    api.post("/restore/resume", {"name": JOB, "target_name": TARGET, "restore_job_id": restore_id})
    api.wait_restore_finishes(JOB, restore_id)
    after = api.restore_report(JOB, restore_id)
    assert after["state"] == "finished", "a refused resume overwrote the stored report: {}".format(after)
    assert after["stats_counters"] == before["stats_counters"]


def test_resume_is_refused_for_read_only_user(api, api_readonly, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    r = api_readonly.raw_post("/restore/resume", {"name": JOB, "target_name": TARGET, "restore_job_id": backup_id})
    assert r.status_code == 403, r.text
