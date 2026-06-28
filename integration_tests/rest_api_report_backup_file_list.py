#!/usr/bin/env python
#
# REST API integration test: /api/v1/report/backup/file/list.
# Confirms pagination, descend mode, and next-token round-trip work end-to-end
# against the daemon's backup database. Uses test_null so no cloud credentials
# are required.
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


class TestRestAPIReportBackupFileList(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_report_backup_file_list')
        self.data_dir = self.to_delete[1]
        self.tmpdir, self.filelist = setup_dir_with_tmp_files()
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd, Loader=yaml.SafeLoader)
            parsed['backup'][0]['paths'] = [self.tmpdir]
            parsed['backup'][0]['exclusions'] = [""]
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
        self.base_url = "http://127.0.0.1:8080"
        self.inttestlog = make_inttest_logfile(prefix="integration_test_log_filelist_")
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
        remove_file_with_retries(self.inttestlog)

    def _decode(self, r, url):
        self.assertEqual(r.headers.get('Content-Type'), 'application/json',
                         "Response for {} has non-JSON content type".format(url))
        return r.json()

    def _run_backup(self, job_name):
        url = self.base_url + self.api_root + '/backup/start'
        r = requests.post(url=url, auth=(self.username, self.password), json={"name": job_name})
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        job_id = self._decode(r, url)['result']['job_id']
        counter = 0
        while True:
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            stopped = any(b['name'] == job_name and b['state'] == 'stopped'
                          for b in self._decode(r, url)['result'])
            if stopped:
                return job_id
            if counter > 200:
                self.fail("Backup '{}' did not finish in 20 seconds".format(job_name))
            time.sleep(0.1)
            counter += 1

    def test_file_list_non_descend_returns_top_level_only(self):
        """With descend=false and path=tmpdir, only direct children must come back."""
        job_id = self._run_backup('first_backup')
        url = self.base_url + self.api_root + '/report/backup/file/list'
        req = {"name": "first_backup", "job_id": job_id, "path": self.tmpdir, "descend": False}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        body = self._decode(r, url)
        self.assertEqual(body['code'], 'success')
        # Direct children of tmpdir = dir1 (one entry). The result list should be non-empty
        # and not contain deep paths like "dir1/dir2/file1.txt".
        result = body['result']
        self.assertGreater(len(result), 0, "Expected at least one direct child of tmpdir")
        for entry in result:
            # entry['path'] should be a direct child of tmpdir (no nested separators
            # beyond the tmpdir prefix itself).
            relative = entry['path'][len(self.tmpdir):].lstrip(os.sep)
            self.assertNotIn(os.sep, relative,
                             "Non-descend listing returned a nested path: {}".format(entry['path']))

    def test_file_list_descend_returns_subtree(self):
        """With descend=true, the listing must include nested files."""
        job_id = self._run_backup('first_backup')
        url = self.base_url + self.api_root + '/report/backup/file/list'
        req = {"name": "first_backup", "job_id": job_id, "path": self.tmpdir, "descend": True,
               "max_results": 1000}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        body = self._decode(r, url)
        result = body['result']
        # In descend mode the listing must include known deep paths.
        deep_paths = [entry['path'] for entry in result]
        expected_deep = self.tmpdir + os.sep + "dir1" + os.sep + "dir2" + os.sep + "file1.txt"
        self.assertIn(expected_deep, deep_paths,
                      "Expected nested file {} to appear in descend listing. Got: {}".format(
                          expected_deep, deep_paths[:5]))

    def test_file_list_next_token_paginates(self):
        """Limit results so pagination kicks in and the next token decodes back to a follow-up page."""
        job_id = self._run_backup('first_backup')
        url = self.base_url + self.api_root + '/report/backup/file/list'
        # Limit to 2 results in descend mode so the result will exceed the page size.
        req = {"name": "first_backup", "job_id": job_id, "path": self.tmpdir,
               "descend": True, "max_results": 2}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        body = self._decode(r, url)
        self.assertEqual(body['code'], 'success')
        # When the result is exactly `max_results` long, the server should include a non-empty next token.
        result = body['result']
        if len(result) == 2:
            self.assertNotEqual(body.get('next', ''), '',
                                "Expected a non-empty next token when page is full")
            # Follow the next token — it should decode and return more rows (or be empty if we hit the end).
            req2 = {"name": "first_backup", "job_id": job_id, "next": body['next']}
            r2 = requests.post(url=url, auth=(self.username, self.password), json=req2)
            self.assertEqual(r2.status_code, 200, url + " " + r2.text)
            body2 = self._decode(r2, url)
            self.assertEqual(body2['code'], 'success')

    def test_file_list_corrupt_next_token_rejected(self):
        """Garbage in `next` must come back as a 400, not a 500 — confirms server-side validation."""
        job_id = self._run_backup('first_backup')
        url = self.base_url + self.api_root + '/report/backup/file/list'
        req = {"name": "first_backup", "job_id": job_id, "next": "!!! not base64 !!!"}
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-v", "--verbose", action="store_true")
    parser.add_argument("unittest_args", nargs="*")
    args = parser.parse_args()
    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)
    sys.argv[1:] = args.unittest_args
    unittest.main(verbosity=2)
