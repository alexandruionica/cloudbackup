#!/usr/bin/env python
"""
User acceptance scenario for the core promise of a backup tool, user guide 6.1/6.2: running the same job
again uploads only what changed, and a later restore reflects the current state of the source.

  run 1  fresh tree            -> everything uploaded
  run 2  nothing changed       -> everything up to date, nothing uploaded
  run 3  one file modified, one added, one deleted
                               -> two uploads, one delete marker, the rest up to date
  restore from run 3           -> modified content, new file present, deleted file absent
  restore from run 1           -> the original tree, deleted file included
"""
import os
import shutil
import tempfile

from common import *


JOB = "first_backup"


def counters(api, job_id):
    return api.backup_report(JOB, job_id)["stats_counters"]


def test_incremental_runs_and_point_in_time_restores(api, client_config, source_tree, restore_dir):
    num_files, num_dirs, _ = count_files_folders_links(source_tree.root)

    # ---- run 1: everything is new
    run1 = api.run_backup_and_wait(JOB)
    check_backup_report(api, JOB, run1, num_files, num_dirs, 0)

    # ---- run 2: nothing changed
    run2 = api.run_backup_and_wait(JOB)
    assert_counters(api, counters(api, run2), {
        "uploaded_files": 0, "uploaded_directories": 0,
        "up_to_date_files": num_files, "up_to_date_directories": num_dirs,
        "marked_deleted_files": 0, "failed_to_upload_files": 0,
    }, "run 2: ")

    # ---- run 3: modify, add, delete
    modified = os.path.join(source_tree.root, "dir1", "dir2", "file1.txt")
    added = os.path.join(source_tree.root, "dir1", "dir5", "file-added-in-run-3.txt")
    deleted = os.path.join(source_tree.root, "dir1", "dir5", "file7.txt")
    original_modified_md5 = get_md5_sum(modified)
    with open(modified, "a", encoding="utf-8") as fd:
        fd.write("\nappended before run 3 so size and mtime both change\n")
    with open(added, "w", encoding="utf-8") as fd:
        fd.write("new in run 3")
    os.remove(deleted)
    run3 = api.run_backup_and_wait(JOB)
    # dir5 gained and lost an entry, so its own mtime changed and it is re-uploaded; dir2 only had a
    # file's content change, which leaves the directory entry untouched
    assert_counters(api, counters(api, run3), {
        "uploaded_files": 2, "uploaded_directories": 1,
        "up_to_date_files": num_files - 2, "up_to_date_directories": num_dirs - 1,
        "marked_deleted_files": 1, "marked_deleted_directories": 0,
        "failed_to_upload_files": 0, "failed_to_find_deleted": 0,
    }, "run 3: ")

    # the CLI report list shows the three runs in start order, all finished
    listed = run_cli("client backup report list {} --json".format(JOB), client_config).json()["result"]
    assert [r["job_id"] for r in listed] == [run1, run2, run3], listed
    assert {r["state"] for r in listed} == {"finished"}

    # ---- restore from run 3: current state of the tree
    current = dict(source_tree.filelist)
    del current[deleted]
    current[added] = "file"
    r3 = api.start_restore(JOB, run3, restore_dir, all_files=True)
    api.wait_restore_finishes(JOB, r3)
    verify_restored_tree(api, restore_dir, source_tree.root, current, absent=[deleted])
    assert get_md5_sum(map_path_into_restore_dir(restore_dir, modified)) == get_md5_sum(modified)
    # the deleted file is present in the run-3 manifest as a delete marker, which the restore skips
    assert_counters(api, api.restore_report(JOB, r3)["stats_counters"], {
        "restored_files": num_files, "restored_directories": num_dirs,
        "skipped_delete_markers": 1, "failed_to_restore_files": 0,
    }, "restore from run 3: ")

    # ---- restore from run 1: the tree as it was, including the since-deleted file and the old content
    restore_dir1 = tempfile.mkdtemp(prefix="integration_test_restore_run1_")
    try:
        r1 = api.start_restore(JOB, run1, restore_dir1, all_files=True)
        api.wait_restore_finishes(JOB, r1)
        for path, kind in source_tree.filelist.items():
            restored = map_path_into_restore_dir(restore_dir1, path)
            assert os.path.exists(restored), "item from run 1 missing in point-in-time restore: {}".format(path)
        assert not os.path.exists(map_path_into_restore_dir(restore_dir1, added)), "run 1 must not contain the run-3 file"
        assert get_md5_sum(map_path_into_restore_dir(restore_dir1, modified)) == original_modified_md5, \
            "restore from run 1 must return the file's original content"
        assert os.path.isfile(map_path_into_restore_dir(restore_dir1, deleted))
    finally:
        shutil.rmtree(restore_dir1, ignore_errors=True)
