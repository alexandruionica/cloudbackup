#!/usr/bin/env python
"""
Scenario for user guide 3 and 8: a configuration change made through the API (what the web UI and
"client config" commands do) is written back to the config file on disk, remains valid for
"server config validate", and is what the daemon runs with after a restart.
"""
import os
import shlex

from common import *


JOB = "first_backup"
NEW_EXCLUSIONS = ["**/*.tmp", "**/cache/**"]


def test_config_change_via_api_survives_daemon_restart(api, daemon, server_config):
    config = api.get("/config")["result"]
    job = next(j for j in config["backup"] if j["name"] == JOB)
    job["exclusions"] = NEW_EXCLUSIONS
    job["checksum"] = True
    api.post("/config", config)

    # on disk, right away
    on_disk = server_config.load()
    disk_job = next(j for j in on_disk["backup"] if j["name"] == JOB)
    assert disk_job["exclusions"] == NEW_EXCLUSIONS and disk_job["checksum"] is True, disk_job
    # secrets must not have been replaced by their masked form when the file was rewritten
    assert on_disk["user"][0]["pass"].startswith("$2"), "bcrypt hash was clobbered on save: {}".format(on_disk["user"][0])
    run_cli("server config validate -c " + shlex.quote(server_config.path))

    # restart on the same port with the same file
    daemon.kill()
    restarted = BackupDaemon(config_path=server_config.path, base_url=daemon.base_url,
                             extra_options="--logfile=" + os.path.join(server_config.data_dir, "restart.log"))
    try:
        after = ApiClient(restarted.base_url).get("/config")["result"]
        job_after = next(j for j in after["backup"] if j["name"] == JOB)
        assert job_after["exclusions"] == NEW_EXCLUSIONS
        assert job_after["checksum"] is True
        # and the daemon still authenticates with the (unmasked) stored hash
        ApiClient(restarted.base_url).get("/backup/list")
    finally:
        restarted.kill()
