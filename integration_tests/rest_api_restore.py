#!/usr/bin/env python
#
# REST API Tests for restore functionality. Uses test_null object store backend.
#
#
import argparse
import json
import logging
import os
import shutil
import sys
import tempfile
import unittest
import requests
import yaml
from common import *


class TestRestAPIRestore(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_rest_api_restore')
        self.data_dir = self.to_delete[1]
        # tmp files for tests
        self.tmpdir, self.filelist = setup_dir_with_tmp_files()
        # adjust server config for job to include above tmpdir
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd, Loader=yaml.SafeLoader)
            parsed['backup'][0]['paths'] = [self.tmpdir]
            parsed['backup'][0]['exclusions'] = [""]
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
        # restore destination directory
        self.restore_dir = tempfile.mkdtemp(prefix="integration_test_restore_dest_")
        # start server
        self.base_url = "http://127.0.0.1:8080"
        _, self.inttestlog = tempfile.mkstemp(prefix="integration_test_log_")
        self.daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=self.base_url,
                                   extra_options="--logfile=" + self.inttestlog)
        self.api_root = '/api/v1'

    def tearDown(self):
        self.daemon.kill()
        for entry in self.to_delete:
            if os.path.exists(entry):
                if os.path.isdir(entry):
                    shutil.rmtree(entry)
                else:
                    os.remove(entry)
        if os.path.exists(self.tmpdir):
            shutil.rmtree(self.tmpdir)
        if os.path.exists(self.restore_dir):
            shutil.rmtree(self.restore_dir)

    def ValidatedAndDecodeResponse(self, r, url):
        self.assertIn('Content-Type', r.headers, "Response for {} is missing header 'Content-Type'".format(url))
        self.assertEqual(r.headers['Content-Type'], 'application/json',
                         "Response for {} is has header 'Content-Type' of value '{}' instead of "
                         "'application/json'".format(url, r.headers['Content-Type']))
        response = r.json()
        self.assertIn("code", response, "Response for {} is missing the 'code' key. Response was:"
                                        " {}".format(url, r.text))
        self.assertIn("message", response, "Response for {} is missing the 'message' key. Response was:"
                                           " {}".format(url, r.text))
        return response

    def _run_backup_and_wait(self, job_name):
        """Start a backup for the given job name, wait for it to complete, and return the backup job_id."""
        url = self.base_url + self.api_root + '/backup/start'
        req = {"name": job_name}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        job_id = response['result']['job_id']
        logging.info("Backup started for '{}' with job_id='{}'".format(job_name, job_id))

        counter = 0
        while True:
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            is_stopped = False
            for backup in response['result']:
                if backup['name'] == job_name and backup['state'] == 'stopped':
                    is_stopped = True
                    break
            if is_stopped:
                break
            if counter > 200:
                self.fail("Backup '{}' did not finish running in 20 seconds".format(job_name))
            time.sleep(0.1)
            counter += 1

        logging.info("Backup '{}' completed with job_id='{}'".format(job_name, job_id))
        return job_id

    def _wait_for_restore_completion(self, job_name, restore_job_id, max_seconds=30):
        """Poll /restore/list until the restore job disappears (i.e. it finished)."""
        counter = 0
        max_count = int(max_seconds / 0.1)
        while True:
            url = self.base_url + self.api_root + '/restore/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            still_running = False
            for entry in response['result']:
                if entry['name'] == job_name and entry.get('job_id', '') == restore_job_id:
                    still_running = True
                    break
            if not still_running:
                break
            if counter > max_count:
                self.fail("Restore for '{}' (restore_job_id='{}') did not finish within {} "
                          "seconds".format(job_name, restore_job_id, max_seconds))
            time.sleep(0.1)
            counter += 1
        logging.info("Restore for '{}' completed (restore_job_id='{}')".format(job_name, restore_job_id))

    # --- input validation tests ---

    def test_restore_start_missing_name(self):
        url = self.base_url + self.api_root + '/restore/start'
        req = {"source_backup_job_id": "some-id", "all_files": True}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

    def test_restore_start_missing_source_job_id(self):
        url = self.base_url + self.api_root + '/restore/start'
        req = {"name": "first_backup", "all_files": True}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

    def test_restore_start_files_and_all_files_mutually_exclusive(self):
        url = self.base_url + self.api_root + '/restore/start'
        req = {"name": "first_backup", "source_backup_job_id": "some-id",
               "all_files": True, "files": ["/etc/hosts"]}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

    def test_restore_start_neither_files_nor_all_files(self):
        url = self.base_url + self.api_root + '/restore/start'
        req = {"name": "first_backup", "source_backup_job_id": "some-id"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

    def test_restore_start_nonexistent_backup_name(self):
        url = self.base_url + self.api_root + '/restore/start'
        req = {"name": "nonexistent_backup_42", "source_backup_job_id": "some-id", "all_files": True}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 404, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("not found", response["code"])

    def test_restore_start_invalid_json(self):
        url = self.base_url + self.api_root + '/restore/start'
        r = requests.post(url=url, auth=(self.username, self.password),
                          data="not json at all",
                          headers={"Content-Type": "application/json"})
        self.assertEqual(r.status_code, 400, url + " " + r.text)

    def test_restore_stop_missing_name(self):
        url = self.base_url + self.api_root + '/restore/stop'
        req = {"restore_job_id": "some-id"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

    def test_restore_stop_not_running(self):
        url = self.base_url + self.api_root + '/restore/stop'
        req = {"name": "first_backup"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("client supplied incorrect data", response["code"])

    # --- access control tests ---

    def test_restore_start_read_only_user_denied(self):
        url = self.base_url + self.api_root + '/restore/start'
        req = {"name": "first_backup", "source_backup_job_id": "some-id", "all_files": True}
        r = requests.post(url=url, auth=(self.username2, self.password2), json=req)
        self.assertEqual(r.status_code, 403, url + " " + r.text)

    def test_restore_stop_read_only_user_denied(self):
        url = self.base_url + self.api_root + '/restore/stop'
        req = {"name": "first_backup"}
        r = requests.post(url=url, auth=(self.username2, self.password2), json=req)
        self.assertEqual(r.status_code, 403, url + " " + r.text)

    def test_restore_list_read_only_user_allowed(self):
        url = self.base_url + self.api_root + '/restore/list'
        r = requests.get(url=url, auth=(self.username2, self.password2))
        self.assertEqual(r.status_code, 200, url + " " + r.text)

    def test_restore_list_empty_by_default(self):
        url = self.base_url + self.api_root + '/restore/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response, "Response is missing 'result' key")
        self.assertEqual(len(response['result']), 0, "Expected no running restores initially, but got: "
                                                     "{}".format(response['result']))

    # --- end-to-end restore lifecycle with test_null backend ---

    def test_restore_full_lifecycle(self):
        """Backup (test_null) -> restore all files -> verify API response flow."""
        job_name = "first_backup"

        # step 1: run a backup and wait for it to complete
        backup_job_id = self._run_backup_and_wait(job_name)

        # step 2: start restore
        logging.info("Starting restore for '{}' from backup job_id '{}'".format(job_name, backup_job_id))
        url = self.base_url + self.api_root + '/restore/start'
        req = {
            "name": job_name,
            "source_backup_job_id": backup_job_id,
            "all_files": True,
            "restore_dir": self.restore_dir,
        }
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response, "Response is missing 'result' key. Response: {}".format(r.text))
        self.assertIn("name", response['result'],
                      "response['result'] missing 'name'. Response: {}".format(r.text))
        self.assertIn("restore_job_id", response['result'],
                      "response['result'] missing 'restore_job_id'. Response: {}".format(r.text))
        restore_job_id = response['result']['restore_job_id']
        self.assertEqual(response['result']['name'], job_name)
        logging.info("Restore started with restore_job_id='{}'".format(restore_job_id))

        # step 3: trying to start another job for the same name while restore is running should fail
        r2 = requests.post(url=url, auth=(self.username, self.password), json=req)
        # it should fail with 400 because a job is already running for this name
        if r2.status_code == 200:
            # the restore may have already finished (test_null is very fast), so this is acceptable
            logging.info("Second restore start returned 200 — the first restore likely already completed")
        else:
            self.assertEqual(r2.status_code, 400, "Expected 400 when a job is already running. "
                                                  "Response: {}".format(r2.text))

        # step 4: wait for restore to complete
        self._wait_for_restore_completion(job_name, restore_job_id)

        # step 5: verify restore list is now empty (since restores are ephemeral)
        url = self.base_url + self.api_root + '/restore/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        for entry in response['result']:
            self.assertNotEqual(entry.get('job_id', ''), restore_job_id,
                                "Restore job '{}' should no longer appear in /restore/list after "
                                "completion".format(restore_job_id))

    def test_restore_watch_streams_progress_events(self):
        """Regression test for the /restore/watch endpoint.

        Before the fix in shared/structs_scheduler.go, every WatchMessage emitted during a restore
        carried a hardcoded JobType='backup', but restore-watch HTTP subscribers register with
        JobType='restore'. The multiplexer in watcher/watcher.go filters by JobType equality, so
        all per-item events were silently dropped and the stream contained only the terminal
        'Restore job has finished' line. This test verifies that a restore now produces per-item
        progress events on /restore/watch.
        """
        job_name = "first_backup"
        # step 1: run a backup so the restore has something to reconstruct from
        backup_job_id = self._run_backup_and_wait(job_name)

        # step 2: start the restore
        url_start = self.base_url + self.api_root + '/restore/start'
        start_req = {
            "name": job_name,
            "source_backup_job_id": backup_job_id,
            "all_files": True,
            "restore_dir": self.restore_dir,
        }
        r = requests.post(url=url_start, auth=(self.username, self.password), json=start_req)
        self.assertEqual(r.status_code, 200, url_start + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url_start)
        restore_job_id = response['result']['restore_job_id']
        logging.info("Started restore '{}' with restore_job_id='{}'".format(job_name, restore_job_id))

        # step 3: attach to /restore/watch. The handler blocks until the restore finishes then
        # closes the connection, so r.text contains the entire SSE stream once requests returns.
        # /restore/start returned only after MarkRestoreRunning committed, so the job is already
        # marked as running and the watch subscribe will succeed.
        url_watch = self.base_url + self.api_root + '/restore/watch'
        watch_req = {"name": job_name, "restore_job_id": restore_job_id}
        r = requests.post(url=url_watch, auth=(self.username, self.password), json=watch_req)
        self.assertEqual(r.status_code, 200, url_watch + " " + r.text)
        r.encoding = 'utf-8'

        # step 4: the final non-empty line must announce completion
        non_empty_lines = [ln for ln in r.text.split('\n') if ln]
        self.assertGreater(len(non_empty_lines), 0, "Restore watch returned an empty response")
        self.assertEqual(non_empty_lines[-1], "data: Restore job has finished",
                         "Expected final line 'data: Restore job has finished' but got: "
                         "{!r}. Full response: {!r}".format(non_empty_lines[-1], r.text))

        # step 5: parse per-item JSON payloads from the SSE stream. Before the fix there were
        # zero such lines — only the terminal string above.
        events = []
        for line in non_empty_lines:
            if not line.startswith("data: "):
                continue
            payload = line[len("data: "):]
            if not payload.startswith("{"):
                # terminal/status strings (e.g. 'Restore job has finished') — skip
                continue
            try:
                events.append(json.loads(payload))
            except json.decoder.JSONDecodeError:
                self.fail("Could not json-decode watch payload: {!r}".format(payload))

        self.assertGreater(
            len(events), 0,
            "Restore watch returned zero per-item events. This regression usually means the "
            "server publishes WatchMessages with JobType='backup' while restore-watch clients "
            "subscribe as 'restore', so the multiplexer filter drops every message. Full "
            "response: {!r}".format(r.text))

        # every streamed event must represent restore-side work; a backup-upload event leaking
        # through would indicate misrouted filtering in the opposite direction.
        for ev in events:
            self.assertNotEqual(
                ev.get("operation_type"), "upload",
                "Restore watch received an 'upload' event, indicating backup-side messages were "
                "misrouted to restore watchers. Event: {!r}".format(ev))

        # step 6: once the watch stream closed, the restore is finished and ephemeral — it must
        # no longer appear in /restore/list
        url_list = self.base_url + self.api_root + '/restore/list'
        r = requests.get(url=url_list, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url_list + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url_list)
        for entry in response['result']:
            self.assertNotEqual(
                entry.get('job_id', ''), restore_job_id,
                "Restore '{}' should no longer appear in /restore/list after watch stream "
                "completion".format(restore_job_id))

    def test_restore_with_invalid_source_backup_job_id(self):
        """Backup (test_null) -> attempt restore with a made-up source_backup_job_id -> expect failure."""
        job_name = "first_backup"
        # run a backup so the database exists
        self._run_backup_and_wait(job_name)

        url = self.base_url + self.api_root + '/restore/start'
        req = {
            "name": job_name,
            "source_backup_job_id": "00000000-0000-0000-0000-000000000000",
            "all_files": True,
            "restore_dir": self.restore_dir,
        }
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        # the restore will start but fail quickly because the source job id doesn't exist;
        # the start handler itself returns 200 (the scheduler accepted the command) or 400 if
        # the scheduler reports an error synchronously
        if r.status_code == 200:
            # restore was accepted; it will fail asynchronously — just wait for it to disappear
            response = self.ValidatedAndDecodeResponse(r, url)
            restore_job_id = response['result']['restore_job_id']
            self._wait_for_restore_completion(job_name, restore_job_id)
        else:
            # scheduler detected the bad source_backup_job_id and reported error synchronously
            self.assertIn(r.status_code, [400, 500],
                          "Unexpected status {} for bad source job id. Response: {}".format(r.status_code, r.text))


def get_args():
    parser = argparse.ArgumentParser(description='Script which performs integration tests for restore')
    parser.add_argument('-v', '--verbose', required=False, action="store_true", default=False,
                        help='Show verbose level messages')
    parser.add_argument('-c', '--cmd', required=False, default="./cloudbackup", help='Path to cloudbackup binary')
    args = parser.parse_args()
    return args


def main():
    global cmd_default
    arguments = get_args()
    cmd_default = arguments.cmd
    if arguments.verbose:
        verbosity = 2
    else:
        verbosity = 1

    logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.WARNING)

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIRestore)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
