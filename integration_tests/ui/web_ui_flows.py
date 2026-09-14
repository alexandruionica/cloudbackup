#!/usr/bin/env python
"""
Browser acceptance tests for the web UI, user guide chapter 7, driven with Playwright against a real
daemon serving /ui/ with a persistent, rate-limited test_null target:

  7.1 connecting              -> Connection dialog, credentials, error on a bad password
  7.2 the job list            -> both jobs listed, start a backup, watch progress, open its report
  7.3 restoring from the UI   -> Restore dialog with a source run, all files, destination override
  7.4 resuming a failed restore -> a cancelled restore shows as resumable and resumes to completion

Run with:  integration_tests/.venv_linux/bin/python -m pytest integration_tests/ui -v
(the runner script installs the Chromium build Playwright needs: ./integration_tests.sh ui)
"""
import os
import re
import time

import pytest
from playwright.sync_api import expect

from common import *


JOB = "first_backup"
USER = "testuser1"
PASSWORD = "HV}H/y?<9$]Z5N4N"


def open_ui(page, daemon, username=USER, password=PASSWORD):
    """Load /ui/ and save credentials through the Connection dialog (the base URL defaults to the page origin)."""
    page.goto(daemon.base_url + "/ui/")
    expect(page.get_by_role("heading", name="CloudBackup")).to_be_visible()
    page.get_by_role("button", name="Connection").click()
    dialog = page.get_by_role("dialog")
    expect(dialog.get_by_role("heading", name="Server connection")).to_be_visible()
    expect(dialog.locator("input[type=url]")).to_have_value(daemon.base_url)
    dialog.locator("input[type=text]").fill(username)
    dialog.locator("input[type=password]").fill(password)
    dialog.get_by_role("button", name="Save").click()
    expect(dialog).to_have_count(0)


def job_card(page, name=JOB):
    return page.locator(".job").filter(has=page.locator("h2", has_text=re.compile("^" + re.escape(name) + "$")))


def wait_for_backup_report(api, job_name, max_seconds=60):
    deadline = time.time() + max_seconds
    while time.time() < deadline:
        reports = api.post("/report/backup/list", {"name": job_name})["result"] or []
        if reports:
            return reports
        time.sleep(0.5)
    raise AssertionError("no backup report appeared within {} seconds".format(max_seconds))


# ---- 7.1 connecting -------------------------------------------------------------------------------------

def test_connect_and_list_jobs(page, daemon):
    open_ui(page, daemon)
    expect(page.locator(".conn-info")).to_contain_text("as " + USER)
    expect(page.locator(".server-version")).to_contain_text("CloudBackup")
    expect(page.locator(".job")).to_have_count(2)
    expect(job_card(page, "first_backup").locator(".state").first).to_have_text("stopped")
    expect(job_card(page, "second_backup").locator(".state").first).to_have_text("stopped")
    expect(page.locator(".error")).to_have_count(0)


def test_wrong_password_shows_an_error_and_no_jobs(page, daemon):
    open_ui(page, daemon, password="not-the-password")
    expect(page.locator(".error")).to_be_visible()
    expect(page.locator(".job")).to_have_count(0)
    # fixing the credentials recovers without a reload
    page.get_by_role("button", name="Connection").click()
    dialog = page.get_by_role("dialog")
    dialog.locator("input[type=password]").fill(PASSWORD)
    dialog.get_by_role("button", name="Save").click()
    expect(page.locator(".job")).to_have_count(2)
    expect(page.locator(".error")).to_have_count(0)


# ---- 7.2 the job list -----------------------------------------------------------------------------------

def test_start_backup_watch_progress_and_open_report(page, daemon, api, source_tree):
    api.set_target_ratelimit(JOB, "300")  # a few seconds of visible progress instead of milliseconds
    open_ui(page, daemon)
    card = job_card(page)
    card.get_by_role("button", name="Start now").click()
    expect(card.locator(".state").first).to_have_text(re.compile("running|started"))

    card.get_by_role("button", name="Watch progress").click()
    dialog = page.get_by_role("dialog")
    expect(dialog.get_by_role("heading", name=re.compile("^Watching:"))).to_be_visible()
    expect(dialog.locator(".evt").first).to_be_visible(timeout=30000)
    assert dialog.locator(".evt .err").count() == 0, "an event carried an error: {}".format(
        dialog.locator(".evt").all_inner_texts())
    dialog.get_by_role("button", name="Close").first.click()  # the watch modal has a Close in header and footer

    api.wait_backup_finishes(JOB)
    reports = wait_for_backup_report(api, JOB)
    job_id = reports[0]["job_id"]
    expect(card.locator(".state").first).to_have_text("stopped", timeout=15000)

    card.get_by_role("button", name="Reports", exact=True).click()
    dialog = page.get_by_role("dialog")
    expect(dialog.get_by_role("heading", name="Reports: " + JOB)).to_be_visible()
    row = dialog.locator("table.report-table tbody tr").first
    expect(row).to_contain_text("finished")
    row.click()
    expect(dialog.get_by_role("heading", name="Report: " + JOB)).to_be_visible()
    expect(dialog.locator(".report-detail")).to_contain_text(job_id)
    num_files, _, _ = count_files_folders_links(source_tree.root)
    assert api.backup_report(JOB, job_id)["stats_counters"]["uploaded_files"] == num_files


# ---- 7.3 restoring from the UI --------------------------------------------------------------------------

def test_restore_all_files_from_the_ui(page, daemon, api, source_tree, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    open_ui(page, daemon)
    card = job_card(page)
    card.get_by_role("button", name="Restore…").click()
    dialog = page.get_by_role("dialog")
    expect(dialog.get_by_role("heading", name="Restore from: " + JOB)).to_be_visible()
    source = dialog.locator("select").first
    expect(source).to_be_enabled()
    source.select_option(backup_id)
    dialog.get_by_label(re.compile("all files")).check()
    dialog.get_by_placeholder(re.compile("^leave empty")).fill(restore_dir)
    dialog.get_by_role("button", name="Start restore").click()
    expect(dialog).to_have_count(0)

    deadline = time.time() + 60
    restore_id = None
    while time.time() < deadline and restore_id is None:
        rows = api.post("/report/restore/list", {"name": JOB})["result"] or []
        for r in rows:
            if r["state"] == "finished":
                restore_id = r["job_id"]
        time.sleep(0.5)
    assert restore_id, "the restore started from the UI never produced a finished report"
    num_files, num_dirs, num_symlinks = verify_restored_tree(api, restore_dir, source_tree.root, source_tree.filelist)
    check_restore_report(api, JOB, restore_id, num_files, num_dirs, num_symlinks)

    card.get_by_role("button", name="Restore reports").click()
    dialog = page.get_by_role("dialog")
    expect(dialog.get_by_role("heading", name="Restore reports: " + JOB)).to_be_visible()
    expect(dialog.locator("table.report-table tbody tr").first).to_contain_text("finished")
    expect(dialog.locator(".resumable-tag")).to_have_count(0)


# ---- 7.4 resuming a failed restore ----------------------------------------------------------------------

def test_cancelled_restore_is_resumable_from_the_ui(page, daemon, api, source_tree, restore_dir):
    backup_id = api.run_backup_and_wait(JOB)
    api.set_target_ratelimit(JOB, "120")
    restore_id = api.start_restore(JOB, backup_id, restore_dir, all_files=True)
    deadline = time.time() + 30
    while time.time() < deadline and not any(files for _, _, files in os.walk(restore_dir)):
        time.sleep(0.1)
    api.post("/restore/stop", {"name": JOB, "restore_job_id": restore_id})
    api.wait_restore_finishes(JOB, restore_id)
    assert api.restore_report(JOB, restore_id)["state"] == "cancelled"
    api.set_target_ratelimit(JOB, "0")

    open_ui(page, daemon)
    job_card(page).get_by_role("button", name="Restore reports").click()
    dialog = page.get_by_role("dialog")
    row = dialog.locator("table.report-table tbody tr").filter(has_text=restore_id)
    expect(row).to_contain_text("cancelled")
    expect(row.locator(".resumable-tag")).to_be_visible()
    row.click()
    expect(dialog.locator(".resume-bar")).to_contain_text("can be resumed")
    dialog.get_by_role("button", name="Resume restore").click()
    expect(dialog).to_have_count(0)

    api.wait_restore_finishes(JOB, restore_id)
    verify_restored_tree(api, restore_dir, source_tree.root, source_tree.filelist)
    assert api.restore_report(JOB, restore_id)["state"] == "finished"
