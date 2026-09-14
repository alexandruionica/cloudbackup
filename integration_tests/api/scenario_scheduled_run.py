#!/usr/bin/env python
"""
Scenario for user guide 3 ("schedule:") and 6.1: a job with a cron schedule runs on its own, without any
client asking for it, and leaves the same report a manual run would. Cron granularity is one minute, so
this test waits for the next minute boundary (up to ~75 s) and is marked slow.
"""
import time

import pytest

from common import *


JOB = "first_backup"


@pytest.mark.slow
def test_cron_schedule_starts_the_job_unattended(api, server_config, source_tree):
    # the daemon fixture already started with the shared config; switch the schedule at runtime the way
    # an operator would, through the config API, so the cron manager reloads
    config = api.get("/config")["result"]
    job = next(j for j in config["backup"] if j["name"] == JOB)
    job["schedule"] = ["* * * * *"]
    api.post("/config", config)

    listed = next(j for j in api.get("/backup/list")["result"] if j["name"] == JOB)
    assert listed.get("next_run"), "backup list must expose next_run for a scheduled job: {}".format(listed)

    deadline = time.time() + 75
    reports = []
    while time.time() < deadline:
        reports = api.post("/report/backup/list", {"name": JOB})["result"] or []
        if reports:
            break
        time.sleep(1)
    assert reports, "no backup run was started by the scheduler within 75 seconds"
    job_id = reports[0]["job_id"]
    api.wait_backup_finishes(JOB)
    num_files, num_dirs, num_symlinks = count_files_folders_links(source_tree.root)
    check_backup_report(api, JOB, job_id, num_files, num_dirs, num_symlinks)
