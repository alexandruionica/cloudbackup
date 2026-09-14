#!/usr/bin/env python
"""
Backup and restore of the shapes real trees have, through the CLI and API, for both values of the
"dereference" setting (user guide 3, "dereference"):

  - symlinks to a file, to a directory, and a dangling one
  - a zero-byte file
  - a file larger than the 5 MiB multipart / block threshold
  - a hard link (two directory entries for one inode)
  - a 200-character file name

With dereference: false the links are stored and restored as links. With dereference: true the link
targets are backed up as regular content and a dangling link is an examine failure, not a silent skip.
"""
import os
import platform
import shlex

import pytest

from common import *


JOB = "first_backup"


@pytest.fixture
def rich_tree():
    root, filelist = setup_dir_with_tmp_files(symlinks=True, empty_file=True, large_file=True, hard_link=True,
                                              long_name=True)
    yield SourceTreeLike(root, filelist)
    import shutil
    shutil.rmtree(root, ignore_errors=True)


class SourceTreeLike(object):
    def __init__(self, root, filelist):
        self.root = root
        self.filelist = filelist
        self.extras = os.path.join(root, "dir1", "extras")

    def has_symlinks(self):
        return any(t == "symlink" for t in self.filelist.values())


def _start(server_config, rich_tree, dereference):
    def patch(parsed):
        parsed["backup"][0]["paths"] = [rich_tree.root]
        parsed["backup"][0]["dereference"] = dereference
    server_config.patch(patch)


def _check_content_shapes(api, restore_dir, rich_tree):
    ex = map_path_into_restore_dir(restore_dir, rich_tree.extras)
    assert os.path.getsize(os.path.join(ex, "empty.bin")) == 0
    assert get_md5_sum(os.path.join(ex, "large.bin")) == get_md5_sum(os.path.join(rich_tree.extras, "large.bin"))
    assert os.path.getsize(os.path.join(ex, "large.bin")) == LARGE_FILE_SIZE
    # the hard link comes back as an ordinary second file with the same bytes
    hl = os.path.join(ex, "hardlink-to-file7.txt")
    assert os.path.isfile(hl) and not os.path.islink(hl)
    assert get_md5_sum(hl) == get_md5_sum(os.path.join(rich_tree.root, "dir1", "dir5", "file7.txt"))
    assert os.path.isfile(os.path.join(ex, ("n" * 196) + ".txt"))


def test_links_preserved_when_dereference_is_off(server_config, rich_tree, restore_dir, request):
    _start(server_config, rich_tree, False)
    daemon = request.getfixturevalue("daemon")
    api = ApiClient(daemon.base_url)
    client_config = write_client_config(daemon.base_url)
    try:
        assert next(b for b in api.get("/config")["result"]["backup"] if b["name"] == JOB)["dereference"] is False, \
            "dereference: false must survive config loading"
        num_files, num_dirs, num_symlinks = count_files_folders_links(rich_tree.root, dereference=False)
        if rich_tree.has_symlinks():
            assert num_symlinks == 3
        job_id = api.run_backup_and_wait(JOB)
        check_backup_report(api, JOB, job_id, num_files, num_dirs, num_symlinks)

        r = run_cli("client restore start {} -i {} --all-files --restore-dir {} -N --json".format(
            JOB, job_id, shlex.quote(restore_dir)), client_config)
        restore_id = r.json()["result"]["restore_job_id"]
        api.wait_restore_finishes(JOB, restore_id)
        verify_restored_tree(api, restore_dir, rich_tree.root, rich_tree.filelist, dereference=False)
        check_restore_report(api, JOB, restore_id, num_files, num_dirs, num_symlinks)
        _check_content_shapes(api, restore_dir, rich_tree)
        if rich_tree.has_symlinks():
            ex = map_path_into_restore_dir(restore_dir, rich_tree.extras)
            # a link to a directory is a link again, not a copied directory
            assert os.path.islink(os.path.join(ex, "link-to-dir5"))
            # and the dangling link is recreated dangling rather than dropped
            assert os.path.islink(os.path.join(ex, "dangling-link"))
            assert not os.path.exists(os.path.join(ex, "dangling-link"))
    finally:
        os.remove(client_config)


def test_links_followed_when_dereference_is_on(server_config, rich_tree, restore_dir, request):
    _start(server_config, rich_tree, True)
    daemon = request.getfixturevalue("daemon")
    api = ApiClient(daemon.base_url)
    num_files, num_dirs, num_symlinks = count_files_folders_links(rich_tree.root, dereference=True)
    assert num_symlinks == 0
    job_id = api.run_backup_and_wait(JOB)
    stats = api.backup_report(JOB, job_id)["stats_counters"]
    expected = {"examined_files": num_files, "uploaded_files": num_files, "examined_directories": num_dirs,
                "uploaded_directories": num_dirs, "examined_symlinks": 0, "uploaded_symlinks": 0,
                "failed_to_upload_files": 0}
    if rich_tree.has_symlinks():
        expected["failed_to_examine"] = 1  # the dangling link has no target to follow
    assert_counters(api, stats, expected, "dereference=true backup: ")

    restore_id = api.start_restore(JOB, job_id, restore_dir, all_files=True)
    api.wait_restore_finishes(JOB, restore_id)
    check_restore_report(api, JOB, restore_id, num_files, num_dirs, 0)
    _check_content_shapes(api, restore_dir, rich_tree)
    if rich_tree.has_symlinks():
        ex = map_path_into_restore_dir(restore_dir, rich_tree.extras)
        # followed links come back as the content they pointed at
        f = os.path.join(ex, "link-to-file1.txt")
        assert os.path.isfile(f) and not os.path.islink(f)
        assert get_md5_sum(f) == get_md5_sum(os.path.join(rich_tree.root, "dir1", "dir2", "file1.txt"))
        d = os.path.join(ex, "link-to-dir5")
        assert os.path.isdir(d) and not os.path.islink(d)
        assert sorted(os.listdir(d)) == ["file7.txt", "file8.htm"]
        assert not os.path.lexists(os.path.join(ex, "dangling-link"))
