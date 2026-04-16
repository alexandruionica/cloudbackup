#!/usr/bin/env python
#
# REST API Tests for restore report functionality (/report/restore/list and /report/restore/show).
# Uses test_null object store backend. Requires a backup+restore cycle to produce report data.
#
#
import argparse
import logging
import os
import shutil
import sys
import tempfile
import unittest
import requests
import yaml
from common import *


class TestRestAPIReportRestore(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_report_restore')
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
                         "Response for {} has header 'Content-Type' of value '{}' instead of "
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

    def _run_restore_and_wait(self, job_name, backup_job_id):
        """Start a restore for the given backup, wait for it to complete, and return the restore job_id."""
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
        restore_job_id = response['result']['restore_job_id']
        logging.info("Restore started for '{}' with restore_job_id='{}'".format(job_name, restore_job_id))

        counter = 0
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
            if counter > 300:
                self.fail("Restore for '{}' (restore_job_id='{}') did not finish within 30 "
                          "seconds".format(job_name, restore_job_id))
            time.sleep(0.1)
            counter += 1

        logging.info("Restore for '{}' completed (restore_job_id='{}')".format(job_name, restore_job_id))
        return restore_job_id

    # --- /report/restore/list input validation ---

    def test_report_restore_list_missing_name(self):
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

    def test_report_restore_list_invalid_json(self):
        url = self.base_url + self.api_root + '/report/restore/list'
        r = requests.post(url=url, auth=(self.username, self.password),
                          data="not json at all",
                          headers={"Content-Type": "application/json"})
        self.assertEqual(r.status_code, 400, url + " " + r.text)

    def test_report_restore_list_invalid_from_start_time(self):
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {"name": "first_backup", "from_start_time": "not-a-date"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)

    def test_report_restore_list_invalid_until_start_time(self):
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {"name": "first_backup", "until_start_time": "not-a-date"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)

    def test_report_restore_list_until_before_from(self):
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {"name": "first_backup",
               "from_start_time": "2026-04-10T00:00:00Z",
               "until_start_time": "2026-04-01T00:00:00Z"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)

    def test_report_restore_list_nonexistent_backup_name(self):
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {"name": "nonexistent_backup_42"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 404, url + " " + r.text)

    def test_report_restore_list_read_only_user_allowed(self):
        """Read-only users should be able to access /report/restore/list."""
        # first run a backup so the database exists
        self._run_backup_and_wait("first_backup")
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {"name": "first_backup"}
        r = requests.post(url=url, auth=(self.username2, self.password2), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)

    # --- /report/restore/show input validation ---

    def test_report_restore_show_missing_name(self):
        url = self.base_url + self.api_root + '/report/restore/show'
        req = {"job_id": "some-id"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

    def test_report_restore_show_missing_job_id(self):
        url = self.base_url + self.api_root + '/report/restore/show'
        req = {"name": "first_backup"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

    def test_report_restore_show_invalid_json(self):
        url = self.base_url + self.api_root + '/report/restore/show'
        r = requests.post(url=url, auth=(self.username, self.password),
                          data="not json at all",
                          headers={"Content-Type": "application/json"})
        self.assertEqual(r.status_code, 400, url + " " + r.text)

    def test_report_restore_show_nonexistent_backup_name(self):
        url = self.base_url + self.api_root + '/report/restore/show'
        req = {"name": "nonexistent_backup_42", "job_id": "some-id"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 404, url + " " + r.text)

    def test_report_restore_show_nonexistent_job_id(self):
        """Show for a non-existent restore job_id should return 404."""
        job_name = "first_backup"
        self._run_backup_and_wait(job_name)
        url = self.base_url + self.api_root + '/report/restore/show'
        req = {"name": job_name, "job_id": "00000000-0000-0000-0000-000000000000"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 404, url + " " + r.text)

    # --- /report/restore/list empty before any restore ---

    def test_report_restore_list_empty_before_any_restore(self):
        """Before any restore has been run, the restore report list should be empty."""
        job_name = "first_backup"
        self._run_backup_and_wait(job_name)
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {"name": job_name}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response,
                      "Response for {} is missing the 'result' key. Response was: {}".format(url, r.text))
        self.assertIn("next", response,
                      "Response for {} is missing the 'next' key. Response was: {}".format(url, r.text))
        self.assertEqual(0, len(response['result']),
                         "Expected empty restore report list before any restore was run. "
                         "Response was: {}".format(r.text))

    # --- end-to-end: backup -> restore -> report list -> report show ---

    def test_report_restore_full_lifecycle(self):
        """Run a backup, then a restore, then verify that /report/restore/list and
        /report/restore/show return the expected data."""
        job_name = "first_backup"

        # step 1: run a backup and wait for it to complete
        backup_job_id = self._run_backup_and_wait(job_name)

        # step 2: run a restore and wait for it to complete
        restore_job_id = self._run_restore_and_wait(job_name, backup_job_id)

        # step 3: /report/restore/list should now contain the completed restore
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {"name": job_name}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response,
                      "Response for {} is missing 'result'. Response: {}".format(url, r.text))
        self.assertIn("next", response,
                      "Response for {} is missing 'next'. Response: {}".format(url, r.text))
        self.assertGreaterEqual(len(response['result']), 1,
                                "Expected at least 1 restore report after running a restore. "
                                "Response: {}".format(r.text))

        # find the restore we just ran
        found = False
        for entry in response['result']:
            if entry['job_id'] == restore_job_id:
                found = True
                self.assertEqual(entry['name'], job_name,
                                 "Expected name='{}', got '{}'".format(job_name, entry['name']))
                self.assertIn(entry['state'], ['finished', 'failed', 'cancelled'],
                              "Expected a terminal state, got '{}'".format(entry['state']))
                self.assertIn('start_time', entry, "Missing 'start_time' in report entry")
                self.assertIn('end_time', entry, "Missing 'end_time' in report entry")
                break
        self.assertTrue(found,
                        "Restore job_id '{}' not found in report list. "
                        "Results: {}".format(restore_job_id, response['result']))

        # step 4: /report/restore/show should return the report for this specific restore
        url = self.base_url + self.api_root + '/report/restore/show'
        req = {"name": job_name, "job_id": restore_job_id}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response,
                      "Response for {} is missing 'result'. Response: {}".format(url, r.text))
        self.assertEqual(response['code'], "success",
                         "Expected code='success', got '{}'".format(response['code']))
        result = response['result']
        self.assertEqual(result['job_id'], restore_job_id,
                         "Expected job_id='{}', got '{}'".format(restore_job_id, result.get('job_id')))
        self.assertIn(result['state'], ['finished', 'failed', 'cancelled'],
                      "Expected a terminal state, got '{}'".format(result['state']))

    def test_report_restore_list_does_not_include_backup_jobs(self):
        """Verify that /report/restore/list does not include backup job reports."""
        job_name = "first_backup"
        backup_job_id = self._run_backup_and_wait(job_name)

        # check that backup report shows the backup
        url = self.base_url + self.api_root + '/report/backup/list'
        req = {"name": job_name}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertGreaterEqual(len(response['result']), 1,
                                "Expected at least 1 backup report. Response: {}".format(r.text))

        # check that restore report list is empty (no restores have been run)
        url = self.base_url + self.api_root + '/report/restore/list'
        req = {"name": job_name}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual(0, len(response['result']),
                         "Restore report list should not include backup jobs. "
                         "Response: {}".format(r.text))

        # also verify the backup job_id is not in the restore list
        for entry in response['result']:
            self.assertNotEqual(entry['job_id'], backup_job_id,
                                "Backup job_id '{}' should not appear in restore report "
                                "list".format(backup_job_id))


def get_args():
    parser = argparse.ArgumentParser(description='Script which performs integration tests for restore reports')
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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIReportRestore)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
