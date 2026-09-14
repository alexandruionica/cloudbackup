#!/usr/bin/env python
"""
User acceptance scenario mirroring user guide chapter 2, "Getting started", sections 2.2 to 2.8, run
end to end on one daemon exactly in the order a new user would type the commands:

  2.2 hash a password                     -> misc hash-password
  2.3 write a minimal server config       -> server config example, edited
  2.4 validate before starting            -> server config validate / dump
  2.5 start the server                    -> server start -c
  2.6 point the CLI client at the server  -> client config example, client server-version
  2.7 test the target, dry-run, back up   -> backup target test / dryrun / start --watch / list / status / report
  2.8 restore something                   -> restore start -i <id> --all-files --watch, default restore dir

The guide's target is AWS S3; this scenario swaps in the persistent test_null target so it runs without
credentials. Everything else is the guide, verbatim.
"""
import bcrypt
import json
import os
import re
import shlex
import shutil
import tempfile

import pytest

from common import *


PASSWORD = "correct horse battery staple"


@pytest.fixture
def workspace():
    root = tempfile.mkdtemp(prefix="integration_test_getting_started_")
    yield root
    shutil.rmtree(root, ignore_errors=True)


def test_getting_started_chapter(workspace):
    # ---- 2.2 hash a password
    proc = run_interactive_shell_cmd(cmd_default + " misc hash-password")
    stdout, _ = proc.communicate(str.encode(PASSWORD + "\n"))
    assert proc.returncode == 0
    m = re.search(r"The hashed password is: (\$2[aby]\$\S+)", stdout.decode("utf-8"))
    assert m, "no bcrypt hash in: {!r}".format(stdout)
    hashed = m.group(1)
    assert bcrypt.checkpw(PASSWORD.encode(), hashed.encode())

    # ---- 2.3 write a minimal server config, starting from the annotated example
    example = run_cli("server config example").stdout
    cfg = yaml.load(example, Loader=yaml.SafeLoader)
    data_dir = os.path.join(workspace, "data")
    tree, filelist = setup_dir_with_tmp_files()
    persist = os.path.join(workspace, "objects")
    os.makedirs(data_dir)
    cfg["data_dir"] = data_dir
    cfg["html_dir"] = "webstatic"
    cfg["https"]["enabled"] = False
    cfg["user"] = [{"name": "admin", "pass": hashed, "access": "write"}]
    cfg["backup"] = [{
        "name": "documents",
        "paths": [tree],
        "target": [{"name": "s3-main", "type": "test_null", "bucket": "my-backup-bucket",
                    "prefix": "backups/server-01",
                    "parameters": [{"name": "persist_dir", "value": persist}]}],
        "schedule": ["30 02 * * *"],
    }]
    cfg.pop("notification", None)
    server_config = os.path.join(workspace, "config.yaml")
    with open(server_config, "w") as fd:
        fd.write(yaml.dump(cfg))

    # ---- 2.4 validate before starting
    run_cli("server config validate -c " + shlex.quote(server_config))
    dumped = run_cli("server config dump -c " + shlex.quote(server_config)).json()
    assert [j["name"] for j in dumped["backup"]] == ["documents"]
    assert dumped["user"][0]["name"] == "admin"
    assert dumped["user"][0]["pass"] != hashed, "secrets must be masked in the dump"

    # ---- 2.5 start the server (foreground form); the harness picks a free port
    daemon = BackupDaemon(config_path=server_config, extra_options="--logfile=" + os.path.join(workspace, "server.log"))
    try:
        # ---- 2.6 point the CLI client at the server
        client_example = yaml.load(run_cli("client config example").stdout, Loader=yaml.SafeLoader)
        client_example.update({"username": "admin", "password": PASSWORD, "address": daemon.base_url})
        client_config = os.path.join(workspace, "client.yaml")
        with open(client_config, "w") as fd:
            fd.write(yaml.dump(client_example))
        run_cli("client config validate", client_config)
        version = run_cli("client server-version", client_config)
        assert "Server version:" in version.stdout, version

        # ---- 2.7 test the target, dry-run, back up
        run_cli("client backup target test documents", client_config)

        dry = run_cli("client backup dryrun documents --json", client_config)
        docs, plain = decode_json_stream(dry.stdout)
        examined = {d["name"] for d in docs if "name" in d and d.get("error", "") == ""}
        assert set(filelist) <= examined, "dry run did not examine every fixture item; missing: {}".format(
            set(filelist) - examined)
        assert any(line.startswith("Completed run:") for line in plain), plain

        watched = run_cli("client backup start documents --watch", client_config)
        assert watched.lines[-1] == "Backup job has finished", watched

        listed = run_cli("client backup list --json", client_config).json()
        job = next(j for j in listed["result"] if j["name"] == "documents")
        assert job["state"] == "stopped"
        status = run_cli("client backup status documents --json", client_config).json()
        assert status["name"] == "documents" and status["state"] == "stopped", status
        assert status["next_run"], "a scheduled job must show its next run"

        reports = run_cli("client backup report list documents --json", client_config).json()["result"]
        assert len(reports) == 1 and reports[0]["state"] == "finished", reports
        backup_id = reports[0]["job_id"]
        shown = run_cli("client backup report show documents -i " + backup_id + " --json", client_config).json()
        num_files, num_dirs, num_symlinks = count_files_folders_links(tree)
        api = ApiClient(daemon.base_url, username="admin", password=PASSWORD)
        assert_counters(api, shown["result"]["stats_counters"],
                        {"uploaded_files": num_files, "uploaded_directories": num_dirs, "failed_to_upload_files": 0},
                        "backup report: ")

        # ---- 2.8 restore something (the guide opens the TUI browser; --all-files is the scripted equivalent)
        restored = run_cli("client restore start documents -i {} --all-files --watch".format(backup_id), client_config)
        # a restore this small can finish before the watch subscription lands; the client then reports the
        # final state from the report instead of streaming progress, and either way exits 0
        assert ("Restore job has finished" in restored.stdout or "Final state: finished" in restored.stdout), restored
        m = re.search(r"Restore job id '([0-9a-f-]{36})'", restored.stdout)
        assert m, restored
        restore_id = m.group(1)
        api.wait_restore_finishes("documents", restore_id)
        # "Restored files land on the server, under <data_dir>/restores/<backup-name>/<restore-job-id>/ by default"
        default_restore_dir = os.path.join(data_dir, "restores", "documents", restore_id)
        assert os.path.isdir(default_restore_dir), "default restore directory missing: {}".format(default_restore_dir)
        verify_restored_tree(api, default_restore_dir, tree, filelist)
        check_restore_report(api, "documents", restore_id, num_files, num_dirs, num_symlinks)
        report = run_cli("client restore report list documents --json", client_config).json()["result"]
        assert [r["job_id"] for r in report] == [restore_id]
    finally:
        daemon.kill()
        shutil.rmtree(tree, ignore_errors=True)
