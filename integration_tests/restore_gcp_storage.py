#!/usr/bin/env python
#
# Integration test: backup to GCP Storage then restore from it and verify files on disk.
#
#
import argparse
import logging
import os
import shutil
import sys
import tempfile
import unittest
import yaml
from common import *


class TestRestoreGcpStorage(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_restore_gcp_storage_')
        # client - config file
        tmphandle, self.client_config_file_path = tempfile.mkstemp(suffix='_integration_tests_client_config_file.yaml')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(working_client_config_file_content)
        tmpfile.close()
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
        if os.path.exists(self.client_config_file_path):
            os.remove(self.client_config_file_path)
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

    def _configure_gcp_target(self, job_name):
        bucket, settings = get_gcp_storage_config_from_env()
        self.assertIsNotNone(bucket, "Environment variable CLD_GCP_STORAGE_BUCKET is not set")

        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200)
        response = r.json()

        job_index = None
        for index, value in enumerate(response['result']['backup']):
            if value['name'] == job_name:
                job_index = index
        self.assertIsNotNone(job_index, "Did not find backup job '{}'".format(job_name))

        response['result']['backup'][job_index]['target'][0]['type'] = "gcp_storage"
        response['result']['backup'][job_index]['target'][0]['bucket'] = bucket
        if self.id().startswith("__main__."):
            prefix = "tests/" + platform.system().lower() + "/" + self.id()[9:]
        else:
            prefix = "tests/" + platform.system().lower() + "/" + self.id()
        response['result']['backup'][job_index]['target'][0]['prefix'] = prefix
        if settings["CLD_GCP_PRIVATE_KEY"]:
            response['result']['backup'][job_index]['target'][0]['parameters'] = [
                {"name": "type", "value": settings["CLD_GCP_TYPE"]},
                {"name": "project_id", "value": settings["CLD_GCP_PROJECT_ID"]},
                {"name": "private_key_id", "value": settings["CLD_GCP_PRIVATE_KEY_ID"]},
                {"name": "private_key", "value": settings["CLD_GCP_PRIVATE_KEY"]},
                {"name": "client_email", "value": settings["CLD_GCP_CLIENT_EMAIL"]},
                {"name": "client_id", "value": settings["CLD_GCP_CLIENT_ID"]},
                {"name": "auth_uri", "value": settings["CLD_GCP_AUTH_URI"]},
                {"name": "token_uri", "value": settings["CLD_GCP_TOKEN_URI"]},
                {"name": "auth_provider_x509_cert_url", "value": settings["CLD_GCP_AUTH_PROVIDER_X509_CERT_URL"]},
                {"name": "client_x509_cert_url", "value": settings["CLD_GCP_CLIENT_X509_CERT_URL"]},
            ]

        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          json=response['result'])
        self.assertEqual(r.status_code, 200, r.text)

        return response['result']['backup'][job_index]

    def _run_backup_and_wait(self, job_name):
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
            if counter > 600:
                self.fail("Backup '{}' did not finish in 60 seconds".format(job_name))
            time.sleep(0.1)
            counter += 1

        logging.info("Backup '{}' completed with job_id='{}'".format(job_name, job_id))
        return job_id

    def _wait_for_restore_completion(self, job_name, restore_job_id, max_seconds=120):
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

    def test_full_backup_and_restore(self):
        bucket, _ = get_gcp_storage_config_from_env()
        self.assertIsNotNone(bucket, "Environment variable CLD_GCP_STORAGE_BUCKET is not set")
        job_name = "first_backup"

        # step 1: configure GCP target and run backup
        backup_cfg = self._configure_gcp_target(job_name)
        expected_num_files, expected_num_dirs, expected_num_symlinks = 0, 0, 0
        for path in backup_cfg['paths']:
            f, d, s = count_files_folders_links(path, backup_cfg["dereference"])
            expected_num_files += f
            expected_num_dirs += d
            expected_num_symlinks += s

        logging.info("Running backup to GCP Storage")
        backup_job_id = self._run_backup_and_wait(job_name)
        check_backup_report(self, job_name, backup_job_id, expected_num_files, expected_num_dirs,
                            expected_num_symlinks)

        # step 2: start restore with all files
        logging.info("Starting restore from GCP backup job_id='{}'".format(backup_job_id))
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
        self.assertIn("result", response)
        restore_job_id = response['result']['restore_job_id']
        logging.info("Restore started with restore_job_id='{}'".format(restore_job_id))

        # step 3: wait for restore to complete
        self._wait_for_restore_completion(job_name, restore_job_id)

        # step 4: verify restored files on disk
        logging.info("Verifying restored files in '{}'".format(self.restore_dir))
        for source_path, file_type in self.filelist.items():
            relative_path = source_path.lstrip(os.sep)
            restored_path = os.path.join(self.restore_dir, relative_path)
            self.assertTrue(os.path.exists(restored_path),
                            "Expected restored item '{}' (type={}) to exist at '{}' but it does "
                            "not".format(source_path, file_type, restored_path))
            if file_type == "dir":
                self.assertTrue(os.path.isdir(restored_path),
                                "Expected '{}' to be a directory".format(restored_path))
            elif file_type == "file":
                self.assertTrue(os.path.isfile(restored_path),
                                "Expected '{}' to be a regular file".format(restored_path))
                original_md5 = get_md5_sum(source_path)
                restored_md5 = get_md5_sum(restored_path)
                self.assertEqual(original_md5, restored_md5,
                                 "MD5 mismatch for '{}': original={} restored={}".format(
                                     source_path, original_md5, restored_md5))

        # step 5: verify restore report counters match what was backed up
        check_restore_report(self, job_name, restore_job_id, expected_num_files, expected_num_dirs,
                             expected_num_symlinks)

        # step 6: count files, directories, and symlinks under the restored subtree and verify the
        # numbers match what was counted on the source side before the backup ran
        restored_source_root = os.path.join(self.restore_dir, self.tmpdir.lstrip(os.sep))
        restored_f, restored_d, restored_s = count_files_folders_links(restored_source_root,
                                                                       backup_cfg["dereference"])
        self.assertEqual(restored_f, expected_num_files,
                         "restored tree at '{}' has {} files but the source had {}".format(
                             restored_source_root, restored_f, expected_num_files))
        self.assertEqual(restored_d, expected_num_dirs,
                         "restored tree at '{}' has {} directories but the source had {}".format(
                             restored_source_root, restored_d, expected_num_dirs))
        self.assertEqual(restored_s, expected_num_symlinks,
                         "restored tree at '{}' has {} symlinks but the source had {}".format(
                             restored_source_root, restored_s, expected_num_symlinks))

    def test_restore_by_directory(self):
        """Restore using a directory path in the 'files' parameter and verify that all contents
        of the directory are restored recursively, while files outside the directory are not."""
        bucket, _ = get_gcp_storage_config_from_env()
        self.assertIsNotNone(bucket, "Environment variable CLD_GCP_STORAGE_BUCKET is not set")
        job_name = "first_backup"

        # step 1: configure GCP target and run backup
        self._configure_gcp_target(job_name)
        logging.info("Running backup to GCP Storage for directory restore test")
        backup_job_id = self._run_backup_and_wait(job_name)

        # step 2: restore only a subdirectory — use a trailing separator to also exercise
        # the server-side path sanitisation
        subdir_to_restore = self.tmpdir + os.sep + "dir1" + os.sep + "dir2" + os.sep
        logging.info("Starting directory restore from GCP for path '{}'".format(subdir_to_restore))
        restore_dir2 = tempfile.mkdtemp(prefix="integration_test_restore_dir_dest_")
        self.to_delete.append(restore_dir2)
        url = self.base_url + self.api_root + '/restore/start'
        req = {
            "name": job_name,
            "source_backup_job_id": backup_job_id,
            "files": [subdir_to_restore],
            "restore_dir": restore_dir2,
        }
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response)
        restore_job_id = response['result']['restore_job_id']
        self._wait_for_restore_completion(job_name, restore_job_id)

        # step 3: verify items inside the requested directory were restored
        subdir_no_trailing = subdir_to_restore.rstrip(os.sep)
        for source_path, file_type in self.filelist.items():
            relative_path = source_path.lstrip(os.sep)
            restored_path = os.path.join(restore_dir2, relative_path)
            is_inside = source_path == subdir_no_trailing or source_path.startswith(subdir_no_trailing + os.sep)
            # Parent directories of the restored subtree are created as a side effect of
            # os.MkdirAll when writing files, so they will exist on disk even though they
            # were not explicitly restored. Skip the "must not exist" check for ancestors.
            is_ancestor = subdir_no_trailing.startswith(source_path + os.sep)
            if is_inside:
                self.assertTrue(os.path.exists(restored_path),
                                "Expected restored item '{}' (type={}) to exist at '{}' but it does "
                                "not".format(source_path, file_type, restored_path))
                if file_type == "dir":
                    self.assertTrue(os.path.isdir(restored_path),
                                    "Expected '{}' to be a directory".format(restored_path))
                elif file_type == "file":
                    self.assertTrue(os.path.isfile(restored_path),
                                    "Expected '{}' to be a regular file".format(restored_path))
                    original_md5 = get_md5_sum(source_path)
                    restored_md5 = get_md5_sum(restored_path)
                    self.assertEqual(original_md5, restored_md5,
                                     "MD5 mismatch for '{}': original={} restored={}".format(
                                         source_path, original_md5, restored_md5))
            elif not is_ancestor:
                # items outside the requested directory (and not ancestors of it) must NOT
                # have been restored
                self.assertFalse(os.path.exists(restored_path),
                                 "Item '{}' is outside the requested directory '{}' but was "
                                 "restored at '{}'".format(source_path, subdir_no_trailing,
                                                           restored_path))


def get_args():
    parser = argparse.ArgumentParser(description='Integration tests for restore from GCP Storage')
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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestoreGcpStorage)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
