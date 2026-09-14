#!/usr/bin/env python
"""
Record a data directory written by a RELEASED cloudbackup binary so the current binary can be tested
against it (integration_tests/api/compat_previous_release.py): can it open the databases, list the old
run's reports and files, run an incremental backup on top, and restore?

Usage, from the repository root, with the old binary built from its tag:

    git worktree add /tmp/cb-v0.0.3 v0.0.3
    (cd /tmp/cb-v0.0.3 && bash generate_version.sh && go build -mod=vendor -o cloudbackup .)
    integration_tests/.venv_linux/bin/python integration_tests/fixtures/make_release_fixture.py \\
        --binary /tmp/cb-v0.0.3/cloudbackup --version v0.0.3

The source tree is created at a fixed path (see TREE_ROOT) because the databases store absolute paths;
the compat test recreates the same tree, content and mtimes there before starting the daemon. The
fixture is therefore tied to the OS family it was generated on and the test skips elsewhere.
"""
import argparse
import json
import os
import platform
import shutil
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "..", "lib"))
from common import *  # noqa: E402

TREE_ROOT = os.path.join(tempfile.gettempdir(), "cloudbackup_compat_fixture")
FIXED_MTIME = 1_700_000_000  # 2023-11-14, constant so the recreated tree matches the recorded metadata

# relative path -> content (None = directory). Deliberately covers the same shapes as the default tree.
TREE = {
    "dir1": None,
    "dir1/dir2": None,
    "dir1/dir2/file1.txt": "some text for file1",
    "dir1/dir2/file2⻆⽄〄㉎㍌㨂侣.html": "<html>unicode name</html>",
    "dir1/dir3ͲᾆЎ": None,
    "dir1/dir3ͲᾆЎ/file5صقڜ.txt": "rtl name",
    "dir1/dir5": None,
    "dir1/dir5/file7.txt": "seven",
    "dir1/dir5/empty.bin": "",
    "dir1/;=&'file9.txt": "metachars",
}


def build_tree(root):
    """(Re)create the fixture tree at $root with constant content and mtimes."""
    if os.path.isdir(root):
        shutil.rmtree(root)
    os.makedirs(root)
    os.utime(root, (FIXED_MTIME, FIXED_MTIME))
    for rel, content in TREE.items():
        path = os.path.join(root, rel)
        if content is None:
            os.makedirs(path, exist_ok=True)
        else:
            with open(path, "w", encoding="utf-8") as fd:
                fd.write(content)
    # directories last (writing children bumps the parent's mtime)
    for rel in sorted(TREE, key=len, reverse=True):
        os.utime(os.path.join(root, rel), (FIXED_MTIME, FIXED_MTIME))
    os.utime(root, (FIXED_MTIME, FIXED_MTIME))
    return {os.path.join(root, rel): ("dir" if c is None else "file") for rel, c in TREE.items()}


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--binary", required=True, help="path to the released cloudbackup binary")
    parser.add_argument("--version", required=True, help="release tag, e.g. v0.0.3; names the fixture directory")
    args = parser.parse_args()

    out_dir = os.path.join(HERE, "release_" + args.version)
    if os.path.isdir(out_dir):
        shutil.rmtree(out_dir)
    os.makedirs(os.path.join(out_dir, "data"))

    build_tree(TREE_ROOT)
    data_dir = tempfile.mkdtemp(prefix="compat_fixture_datadir_")
    config_path = os.path.join(out_dir, "config.yaml")
    with open(config_path, "w") as fd:
        fd.write(working_server_config_file_content.replace("data_dir: ./tmp/", "data_dir: " + data_dir, 1))
    with open(config_path) as fd:
        parsed = yaml.load(fd, Loader=yaml.SafeLoader)
    parsed["backup"] = [parsed["backup"][0]]
    parsed["backup"][0]["paths"] = [TREE_ROOT]
    parsed["backup"][0]["exclusions"] = [""]
    with open(config_path, "w") as fd:
        fd.write(yaml.dump(parsed))

    daemon = BackupDaemon(config_path=config_path, cmd=os.path.abspath(args.binary))
    try:
        api = ApiClient(daemon.base_url)
        version = api.get("/report/version")["result"]
        job_id = api.run_backup_and_wait("first_backup")
        report = api.backup_report("first_backup", job_id)
        assert report["state"] == "finished", report
    finally:
        daemon.kill()

    for name in os.listdir(data_dir):
        if name.endswith(".sqlite"):
            shutil.copy2(os.path.join(data_dir, name), os.path.join(out_dir, "data", name))
    shutil.rmtree(data_dir, ignore_errors=True)
    # the committed config must not point at the throwaway data dir
    parsed["data_dir"] = "REPLACED_BY_TEST"
    with open(config_path, "w") as fd:
        fd.write(yaml.dump(parsed))

    meta = {
        "version": args.version,
        "server_version": version,
        "platform": platform.system(),
        "tree_root": TREE_ROOT,
        "fixed_mtime": FIXED_MTIME,
        "tree": TREE,
        "job_id": job_id,
        "stats_counters": report["stats_counters"],
    }
    with open(os.path.join(out_dir, "meta.json"), "w", encoding="utf-8") as fd:
        json.dump(meta, fd, indent=2, ensure_ascii=False, sort_keys=True)
    shutil.rmtree(TREE_ROOT, ignore_errors=True)
    print("fixture written to", out_dir)
    print("files:", sorted(os.listdir(os.path.join(out_dir, "data"))))


if __name__ == "__main__":
    main()
