#!/usr/bin/env python
#
# Integration test: client-side-encryption lifecycle exercised through the daemon's
# REST API. Uses the test_null object store backend.
#
# The test_null target runs with a persist_dir so the ciphertext and the keystore
# sidecar written by the backup job survive into the restore job, which lets this
# test prove the full encrypt -> upload -> download -> decrypt round-trip through
# the daemon:
#
#   - daemon starts cleanly with encrypt=true, encrypt_pass=<value>
#   - POST /backup/target/test succeeds (sidecar bootstrap path runs cleanly)
#   - GET /config shows encrypt=true and a populated (obfuscated) encrypt_pass
#   - POST /backup/start completes the job and the report exposes all of the CSE
#     stats counters at zero
#   - POST /restore/start restores every file byte-for-byte and the restore
#     report's CSE counter (decrypt_keystore_mismatch) is present and zero
#
# Cross-daemon behaviour (sidecar rehydrate, conflict resolution, reset-keystore
# CLI) is covered by the Go unit tests in objectstore/encryption_test.go,
# objectstore/store_test_null_persist_test.go and cliargs/cliargs_resetkeystore_test.go.

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


ENCRYPT_PASS = 'integration-test-encryption-pass-1'


class TestEncryptionLifecycle(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.base_url = free_base_url()
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_encryption_lifecycle', persistent_null_store=True)
        self.data_dir = self.to_delete[1]
        # Tmp source tree to back up.
        self.tmpdir, self.filelist = setup_dir_with_tmp_files()
        # Patch the loaded server config: first_backup gets the tmp source tree and
        # encryption enabled with a known password.
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd, Loader=yaml.SafeLoader)
            parsed['backup'][0]['paths'] = [self.tmpdir]
            parsed['backup'][0]['exclusions'] = [""]
            parsed['backup'][0]['encrypt'] = True
            parsed['backup'][0]['encrypt_pass'] = ENCRYPT_PASS
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
        # Restore destination.
        self.restore_dir = tempfile.mkdtemp(prefix="integration_test_encryption_restore_")
        # Start daemon.
        self.inttestlog = make_inttest_logfile(prefix="integration_test_log_encryption_")
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
        remove_file_with_retries(self.inttestlog)

    def _decode(self, r, url):
        self.assertIn('Content-Type', r.headers,
                      "Response for {} is missing header 'Content-Type'".format(url))
        self.assertEqual(r.headers['Content-Type'], 'application/json',
                         "Response for {} has Content-Type '{}'".format(url, r.headers['Content-Type']))
        response = r.json()
        self.assertIn("code", response)
        self.assertIn("message", response)
        return response

    # the shared report helpers in common.py expect this name
    ValidatedAndDecodeResponse = _decode

    def _wait_backup_finishes(self, job_name, max_seconds=20):
        counter = 0
        max_count = int(max_seconds / 0.1)
        while True:
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self._decode(r, url)
            stopped = False
            for backup in response['result']:
                if backup['name'] == job_name and backup['state'] == 'stopped':
                    stopped = True
                    break
            if stopped:
                return
            if counter > max_count:
                self.fail("Backup '{}' did not finish in {} seconds".format(job_name, max_seconds))
            time.sleep(0.1)
            counter += 1

    def _wait_restore_finishes(self, job_name, restore_job_id, max_seconds=30):
        counter = 0
        max_count = int(max_seconds / 0.1)
        while True:
            url = self.base_url + self.api_root + '/restore/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self._decode(r, url)
            still_running = False
            for entry in response['result']:
                if entry['name'] == job_name and entry.get('job_id', '') == restore_job_id:
                    still_running = True
                    break
            if not still_running:
                return
            if counter > max_count:
                self.fail("Restore for '{}' (restore_job_id='{}') did not finish in {} "
                          "seconds".format(job_name, restore_job_id, max_seconds))
            time.sleep(0.1)
            counter += 1

    def test_encrypted_backup_and_restore_round_trip(self):
        """Encrypted backup then restore through the daemon returns the original bytes."""
        job_name = 'first_backup'

        # 1. Sanity check: the running config reflects encrypt=true with a populated password.
        url = self.base_url + self.api_root + '/config'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        cfg = self._decode(r, url)
        backup_cfg = next(b for b in cfg['result']['backup'] if b['name'] == job_name)
        self.assertTrue(backup_cfg.get('encrypt'),
                        "Expected encrypt=true on running config for '{}'".format(job_name))
        # The server obfuscates passwords on read so we cannot assert exact value;
        # we only assert it is non-empty.
        self.assertTrue(backup_cfg.get('encrypt_pass'),
                        "Expected non-empty encrypt_pass on running config")

        # 2. Backup.
        url = self.base_url + self.api_root + '/backup/start'
        r = requests.post(url=url, auth=(self.username, self.password), json={"name": job_name})
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        backup_job_id = self._decode(r, url)['result']['job_id']
        self._wait_backup_finishes(job_name)

        # 3. Report — verify CSE counters and that the job did real work.
        url = self.base_url + self.api_root + '/report/backup/show'
        r = requests.post(url=url, auth=(self.username, self.password),
                          json={"name": job_name, "job_id": backup_job_id})
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        report = self._decode(r, url)['result']
        self.assertEqual(report['state'], 'finished',
                         "Expected backup state=finished, got: {}".format(report['state']))
        stats = report['stats_counters']
        # Job did real work — uploaded some files.
        self.assertGreater(stats['uploaded_files'], 0,
                           "Expected uploaded_files>0; report stats: {}".format(stats))
        # All CSE counters must be present and zero on the happy path.
        for counter in ['skipped_reserved_path', 'skipped_too_large_for_target',
                        'keystore_inconsistent']:
            self.assertIn(counter, stats,
                          "Backup report missing CSE counter '{}'. stats: {}".format(counter, stats))
            self.assertEqual(stats[counter], 0,
                             "Expected CSE counter '{}'=0, got {}".format(counter, stats[counter]))

        # 4. Restore everything from that run into an empty directory.
        url = self.base_url + self.api_root + '/restore/start'
        r = requests.post(url=url, auth=(self.username, self.password),
                          json={"name": job_name, "source_backup_job_id": backup_job_id,
                                "all_files": True, "restore_dir": self.restore_dir})
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        restore_job_id = self._decode(r, url)['result']['restore_job_id']
        self._wait_restore_finishes(job_name, restore_job_id)

        # 5. Every file decrypts back to the exact source bytes.
        num_files, num_dirs, num_symlinks = verify_restored_tree(self, self.restore_dir, self.tmpdir,
                                                                 self.filelist)
        check_restore_report(self, job_name, restore_job_id, num_files, num_dirs, num_symlinks)

        # 6. The restore report carries the CSE counter and it is zero: the sidecar the restore job
        #    rehydrated is the one the backup job bootstrapped.
        url = self.base_url + self.api_root + '/report/restore/show'
        r = requests.post(url=url, auth=(self.username, self.password),
                          json={"name": job_name, "job_id": restore_job_id})
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        stats = self._decode(r, url)['result']['stats_counters']
        self.assertIn('decrypt_keystore_mismatch', stats,
                      "Restore report missing CSE counter 'decrypt_keystore_mismatch'. stats: {}".format(stats))
        self.assertEqual(stats['decrypt_keystore_mismatch'], 0)

    def test_target_test_succeeds_with_encryption_enabled(self):
        """POST /backup/target/test must succeed for an encrypted job (sidecar bootstrap)."""
        url = self.base_url + self.api_root + '/backup/target/test'
        r = requests.post(url=url, auth=(self.username, self.password),
                          json={"name": 'first_backup'})
        # 200 means the target validated AND the keystore sidecar lifecycle
        # (InitEncryption with AllowBootstrap=true) completed successfully.
        self.assertEqual(r.status_code, 200, url + " " + r.text)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-v", "--verbose", action="store_true", help="enable verbose logging")
    parser.add_argument("unittest_args", nargs="*")
    args = parser.parse_args()
    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)
    sys.argv[1:] = args.unittest_args
    unittest.main(verbosity=2)
