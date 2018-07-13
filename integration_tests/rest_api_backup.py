#!/usr/bin/env python
#
# CLI Tests which require a server to be running as a prerequisite of the test
#
#
import argparse
import bcrypt
import copy
import json
import logging
import os
import re
import shutil
import sys
import tempfile
import unittest
import requests
import yaml
from common import *
from pprint import pprint


class TestRestAPIBackup(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        tmphandle, self.server_config_file_path = tempfile.mkstemp(suffix='_integration_tests_rest_api_backup.yaml')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(working_server_config_file_content)
        tmpfile.close()
        # tmp files for tests
        self.tmpdir, self.filelist = setup_dir_with_tmp_files()
        # adjust server config for job to include above tmpdir
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd)
            parsed['backup'][0]['paths'] = [self.tmpdir]
            parsed['backup'][0]['exclusions'] = [self.tmpdir + os.sep + "dir1" + os.sep + "dir5", '**' + os.sep +
                                                 'file9.txt']
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
        # start server
        self.base_url = "http://127.0.0.1:8080"
        self.daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=self.base_url)
        self.api_root = '/api/v1'

    def tearDown(self):
        if os.path.exists(self.server_config_file_path):
            os.remove(self.server_config_file_path)
        self.daemon.kill()
        if os.path.exists(self.tmpdir):
            shutil.rmtree(self.tmpdir)

    def ValidatedAndDecodeResponse(self, r, url):
        """
        Checks for the standard stuff we expect in any api response. Returns json decoded response
        :param r: object returned by requests.get()
        :param url: url which was requested (used for error messages)
        :return: json decoded response from requests.get()
        """
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

    # check list of backup jobs
    def test_get_backup_jobs_list(self):
        r = requests.get(self.base_url + self.api_root + '/backup/list', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/backup/list'))
        # check if response can be JSON decoded
        response = r.json()
        # check response has expected keys
        self.assertIn("code", response, "Response is missing the 'code' key. Response was: {}".format(r.text))
        self.assertIn("message", response, "Response is missing the 'message' key. Response was: {}".format(r.text))
        self.assertIn("result", response, "Response is missing the 'result' key. Response was: {}".format(r.text))
        self.assertGreaterEqual(2, len(response['result']), "'result' key should have at least 2 results contained. "
                                                            "Response was: {}".format(r.text))
        self.assertIn("name", response['result'][0], "response['result'][0] is missing the 'name' key. Response was: "
                                                     "{}".format(r.text))
        self.assertIn("state", response['result'][0], "response['result'][0] is missing the 'state' key. Response was: "
                                                      "{}".format(r.text))
        self.assertIn("start_time", response['result'][0], "response['result'][0] is missing the 'start_time' key. "
                                                           "Response was: {}".format(r.text))
        self.assertIn("next_run", response['result'][0], "response['result'][0] is missing the 'next_run' key. "
                                                         "Response was: {}".format(r.text))

    # starts a backup job and then stops it
    def test_backup_job_start_stop(self):
        # fetch list of jobs and start the first one
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        job_name = response['result'][0]['name']
        logging.info("Starting backup for job: {}".format(job_name))

        # attempt to start backup using a user having only read-only access (should not be able to start backup)
        req = {"name": job_name}
        url = self.base_url + self.api_root + '/backup/start'
        r = requests.post(url=url, auth=(self.username2, self.password2), json=req)
        self.assertEqual(r.status_code, 403, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)

        # attempt to start backup with user having correct privileges
        url = self.base_url + self.api_root + '/backup/start'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("name", response['result'], "For {} response['result'] is missing the 'name' key. Response was:"
                                                  " {}".format(url, r.text))
        self.assertIn("job_id", response['result'], "For {} response['result'] is missing the 'job_id' key. Response "
                                                    "was: {}".format(url, r.text))
        job_id = response['result']['job_id']

        # fetch again list of jobs and check that status of job is now "running"
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        is_running = False
        job_id_matches = False
        found_job_id = ""
        for backup in response['result']:
            if backup['name'] == job_name and backup['state'] == 'running':
                is_running = True
                if backup['job_id'] == job_id:
                    job_id_matches = True
                else:
                    found_job_id = backup['job_id']
        self.assertTrue(is_running, "did not manage to find a running backup for job having name: '{}'. "
                                    "Response from server was: {}".format(job_name, r.text))
        self.assertTrue(job_id_matches, "While job named '{}' is running, the job id does not match. Expected to find"
                                        "job id '{}' but found instead '{}'. Full response is:"
                                        " {}".format(job_name, job_id, found_job_id, r.text))
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertGreaterEqual(2, len(response['result']), "for {} 'result' key should have at least 2 results "
                                                            "contained. Response was: {}".format(url, r.text))
        self.assertIn("name", response['result'][0], "for {} response['result'][0] is missing the 'name' key. "
                                                     "Response was: {}".format(url, r.text))
        self.assertIn("state", response['result'][0], "for {} response['result'][0] is missing the 'state' key. "
                                                      "Response was: {}".format(url, r.text))
        self.assertIn("start_time", response['result'][0], "for {} response['result'][0] is missing the 'start_time' "
                                                           "key. Response was: {}".format(url, r.text))
        self.assertIn("next_run", response['result'][0], "for {} response['result'][0] is missing the 'next_run' key. "
                                                         "Response was: {}".format(url, r.text))

        # attempt to stop backup using user which doesn't have the right privileges
        req = {"name": job_name}
        url = self.base_url + self.api_root + '/backup/stop'
        r = requests.post(url=url, auth=(self.username2, self.password2), json=req)
        self.assertEqual(r.status_code, 403, url + " " + r.text)
        self.assertIn("code", response, "Response for {} is missing the 'code' key. Response was:"
                                        " {}".format(url, r.text))
        self.assertIn("message", response, "Response for {} is missing the 'message' key. Response was:"
                                           " {}".format(url, r.text))

        # attempt to stop backup using user which has the right privileges
        req = {"name": job_name}
        url = self.base_url + self.api_root + '/backup/stop'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("name", response['result'], "For {} response['result'] is missing the 'name' key. Response was:"
                                                  " {}".format(url, r.text))
        self.assertIn("job_id", response['result'], "For {} response['result'] is missing the 'job_id' key. Response "
                                                    "was: {}".format(url, r.text))

        # list again jobs and check that for the stopped job state is one of "stopping" or "stopped"
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = r.json()
        is_stopping_or_stopped = False
        for backup in response['result']:
            if backup['name'] == job_name and (backup['state'] == 'stopping' or backup['state'] == 'stopped'):
                is_stopping_or_stopped = True
        self.assertTrue(is_stopping_or_stopped, "did not manage to find a backup for job having name: '{}' and state "
                                                "one of 'stopping' or 'stopped'. Response from server "
                                                "was: {}".format(job_name, r.text))

    # starts a backup job and then stops it
    def test_backup_job_start_stop2(self):
        # fetch list of jobs and start the first one
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        job_name = response['result'][0]['name']
        job2_name = response['result'][1]['name']
        logging.info("Starting backup for job: {}".format(job_name))

        # attempt to start backup with user having correct privileges but using an incorrect format
        req = {"nameASD": job_name}
        url = self.base_url + self.api_root + '/backup/start'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("invalid json", response["code"])

        # attempt to start backup with user having correct privileges but using inexisting job name
        req = {"name": '345sdf-0213odas-323'}
        url = self.base_url + self.api_root + '/backup/start'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 404, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("not found", response["code"])

        # attempt to start backup with user having correct privileges
        req = {"name": job_name}
        url = self.base_url + self.api_root + '/backup/start'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("name", response['result'], "For {} response['result'] is missing the 'name' key. Response was:"
                                                  " {}".format(url, r.text))
        self.assertIn("job_id", response['result'], "For {} response['result'] is missing the 'job_id' key. Response "
                                                    "was: {}".format(url, r.text))
        job_id = response['result']['job_id']

        # fetch again list of jobs and check that status of job is now "running"
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertGreaterEqual(2, len(response['result']), "for {} 'result' key should have at least 2 results "
                                                            "contained. Response was: {}".format(url, r.text))
        self.assertIn("name", response['result'][0], "for {} response['result'][0] is missing the 'name' key. "
                                                     "Response was: {}".format(url, r.text))
        self.assertIn("state", response['result'][0], "for {} response['result'][0] is missing the 'state' key. "
                                                      "Response was: {}".format(url, r.text))
        self.assertIn("start_time", response['result'][0], "for {} response['result'][0] is missing the 'start_time' "
                                                           "key. Response was: {}".format(url, r.text))
        self.assertIn("next_run", response['result'][0], "for {} response['result'][0] is missing the 'next_run' key. "
                                                         "Response was: {}".format(url, r.text))
        is_running = False
        job_id_matches = False
        found_job_id = ""
        for backup in response['result']:
            if backup['name'] == job_name and backup['state'] == 'running':
                is_running = True
                if backup['job_id'] == job_id:
                    job_id_matches = True
                else:
                    found_job_id = backup['job_id']
        self.assertTrue(is_running, "did not manage to find a running backup for job having name: '{}'. "
                                    "Response from server was: {}".format(job_name, r.text))
        self.assertTrue(job_id_matches, "While job named '{}' is running, the job id does not match. Expected to find"
                                        "job id '{}' but found instead '{}'. Full response is:"
                                        " {}".format(job_name, job_id, found_job_id, r.text))

        # attempt to stop backup using user which has the right privileges but calling an inexisting job name
        req = {"name": "asdasd23das34asdas23"}
        url = self.base_url + self.api_root + '/backup/stop'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("client supplied incorrect data", response["code"])

        # attempt to stop backup using user which has the right privileges but calling an inexisting job name and
        # inexisting job id
        req = {"name": "asdasd23das34asdas23",
               "job_id": 'blabla'}
        url = self.base_url + self.api_root + '/backup/stop'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("client supplied incorrect data", response["code"])

        # attempt to stop backup using user which has the right privileges but calling an incorrect job id
        req = {"name": job_name,
               "job_id": 'blabla'}
        url = self.base_url + self.api_root + '/backup/stop'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("client supplied incorrect data", response["code"])

        # attempt to stop backup using user which has the right privileges but calling a job name which isn't running
        # now
        req = {"name": job2_name}
        url = self.base_url + self.api_root + '/backup/stop'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 400, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertEqual("client supplied incorrect data", response["code"])

        # attempt to stop backup using user which has the right privileges and using a correct job_id
        req = {"name": job_name,
               "job_id": job_id}
        url = self.base_url + self.api_root + '/backup/stop'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("name", response['result'], "For {} response['result'] is missing the 'name' key. Response was:"
                                                  " {}".format(url, r.text))
        self.assertIn("job_id", response['result'], "For {} response['result'] is missing the 'job_id' key. Response "
                                                    "was: {}".format(url, r.text))
        # list again jobs and check that for the stopped job state is one of "stopping" or "stopped"
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertGreaterEqual(2, len(response['result']), "for {} 'result' key should have at least 2 results "
                                                            "contained. Response was: {}".format(url, r.text))
        self.assertIn("name", response['result'][0], "for {} response['result'][0] is missing the 'name' key. "
                                                     "Response was: {}".format(url, r.text))
        self.assertIn("state", response['result'][0], "for {} response['result'][0] is missing the 'state' key. "
                                                      "Response was: {}".format(url, r.text))
        self.assertIn("start_time", response['result'][0], "for {} response['result'][0] is missing the 'start_time' "
                                                           "key. Response was: {}".format(url, r.text))
        self.assertIn("next_run", response['result'][0], "for {} response['result'][0] is missing the 'next_run' key. "
                                                         "Response was: {}".format(url, r.text))
        is_stopping_or_stopped = False
        for backup in response['result']:
            if backup['name'] == job_name and (backup['state'] == 'stopping' or backup['state'] == 'stopped'):
                is_stopping_or_stopped = True
        self.assertTrue(is_stopping_or_stopped, "did not manage to find a backup for job having name: '{}' and state "
                                                "one of 'stopping' or 'stopped'. Response from server "
                                                "was: {}".format(job_name, r.text))

    # dryruns a backup job ; examines the whole reply in a non-streaming mode
    def test_backup_job_dryrun1(self):
        # fetch list of jobs and start the first one
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        job_name = response['result'][0]['name']
        logging.info("DryRunning backup for job: {}".format(job_name))

        # attempt to dryrun backup using a user having only read-only access (should be able to dryrun backup)
        req = {"name": job_name}
        url = self.base_url + self.api_root + '/backup/dryrun'
        r = requests.post(url=url, auth=(self.username2, self.password2), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        r.encoding = 'utf-8'

        num_files, num_dirs = 0, 0
        for fname in self.filelist:
            if self.filelist[fname] == "dir":
                num_dirs += 1
            elif self.filelist[fname] == "file":
                num_files += 1
        last_line = r.text.split('\n')[-2]
        re_result = re.search('^data: Completed run: ([0-9]+) examined files, ([0-9]+) examined directories, ([0-9]+) '
                              'excluded files or directories, ([0-9]+) errors encountered', last_line)
        # check regex worked
        self.assertTrue(re_result)
        examined_files = int(re_result.group(1))
        examined_directories = int(re_result.group(2))
        excluded_files_or_dirs = int(re_result.group(3))
        errors_encountered = int(re_result.group(4))

        # decode output except last line
        dryrun_examined = {}
        not_decoded = 0
        confirmed_excluded_files_or_dirs = 0
        excluded_elements = []
        for line in r.text.split('\n'):
            try:
                # a line looks like:
                #  data: {"name":"/tmp/integration_test_czo1dexh","type":"directory","excluded":false,"exclusion_expr":"","error":""}
                # so we will ignore the first 6 characters
                decoded = json.loads(line[5:])
            except json.decoder.JSONDecodeError:
                not_decoded += 1
                continue
            if decoded['excluded']:
                confirmed_excluded_files_or_dirs += 1
                excluded_elements.append(decoded["name"])
                continue
            if decoded['type'] == "directory":
                dryrun_examined[decoded["name"]] = 'dir'
            else:
                dryrun_examined[decoded["name"]] = decoded['type']
        # remove excluded items from the initial dir we use for comparison
        filelist_copy = copy.copy(self.filelist)
        for element in excluded_elements:
            for item in self.filelist:
                if element == item or item.startswith(element + os.sep):
                    filelist_copy.pop(item)
                    continue
        self.assertEqual(2, not_decoded, "More than two lines in the response could not be json decoded. It is expected"
                                         " that 1 line starting with 'Completed run:' and the last (empty) line can't"
                                         " be json decoded")
        # add to the list of generated files also the top level dir. This because the dryrun will include it
        filelist_copy[self.tmpdir] = 'dir'
        # in case the dicts don't match, show the full diff
        self.maxDiff = None
        self.assertDictEqual(filelist_copy, dryrun_examined)
        # we've excluded 1 folder containing 2 files and also separately excluded 1 file so we know for sure 3 less
        #   files should have been reported
        self.assertEqual(num_files - 3, examined_files)
        # top level dir counts too so we increment with 1 the initial list of directories
        # we've excluded 1 folder containing so we know for sure 1 less folder should have been reported
        self.assertEqual(num_dirs + 1 - 1, examined_directories)
        # we've excluded 1 folder containing 2 files and also separately excluded 1 file. The below counter should
        # not be 4 but 2 because the 2 files contained within the excluded directory  ever got looked at because
        # the folder which was excluded wasn't descended into
        self.assertEqual(2, excluded_files_or_dirs)
        self.assertEqual(0, errors_encountered)


def get_args():
    """ Get arguments from CLI """

    parser = argparse.ArgumentParser(description='Script which performs unit tests')
    parser.add_argument('-v', '--verbose', required=False, action="store_true", default=False,
                        help='Show verbose level messages')
    parser.add_argument('-c', '--cmd', required=False, default="./cloudbackup", help='Path to cloudbackup binary')
    args = parser.parse_args()
    return args


def main():
    # noinspection PyGlobalUndefined
    global cmd_default
    arguments = get_args()
    cmd_default = arguments.cmd
    if arguments.verbose:
        verbosity = 2
    else:
        verbosity = 1

    logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.WARNING)

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIBackup)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
