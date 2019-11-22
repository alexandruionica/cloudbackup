#!/usr/bin/env python
#
# API Tests which require a server to be running as a prerequisite of the test
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
import stat
import sqlite3
import sys
import tempfile
import unittest
import requests
import yaml
from common import *
from pprint import pprint

UNIX_SCRIPT = """#!/bin/sh
# arguments are supposed to be JobId
if [ $# -ne 1 ]; then
    echo "expected 1 arguments but got $#"
    echo -n "received arguments were: "
    for var in "$@"; do
        echo -n "\"$var\" "
    done
    echo ""
    exit 1
fi
sleep 1
"""

WINDOWS_SCRIPT = """$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
if ( $args.Count -ne 1 ) {
    Write-Host "You passed $($args.Count) arguments but only 1 was expected. Arguments passed(one per line):"
    $args | Write-Host
    exit 1
}
Start-Sleep -s 1
"""


class TestRestAPIBackupRunScripts(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        fh, self.script_result_path = tempfile.mkstemp(suffix="_integration_tests_backup_pre_post_run_run_"
                                                              "script_output.txt")
        os.close(fh)
        # setup notification script
        if platform.system().lower() == 'windows':
            additional_script_code = """
$i = 0
$found = 0
while($i -lt 50)
{{
    Try
        {{
        $args[0] | Out-File -Encoding ASCII -FilePath {}
        $found = 1
        break
        }}
    Catch
        {{
        $i++
        Write-Host "Sleeping 0.1 seconds while waiting for the file lock to be removed"
        Start-Sleep -m 100
        }}
}}
if ( $found -ne 1 ) {{
    Write-Host "Did not manage to open and write to the output file"
    exit 1
}}
""".format(self.script_result_path)
            self.script = WINDOWS_SCRIPT + additional_script_code
            script_suffix = "_integration_tests_backup_pre_run_notification_notification_script.ps1"
        else:
            self.script = UNIX_SCRIPT + "echo $1 > {}".format(self.script_result_path)
            script_suffix = "_integration_tests_backup_pre_run_notification_notification_script.sh"
        tmphandle, self.script_path = tempfile.mkstemp(suffix=script_suffix)
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(self.script)
        tmpfile.close()
        # make script executable
        st = os.stat(self.script_path)
        os.chmod(self.script_path, st.st_mode | stat.S_IEXEC)
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_rest_api_backup_pre_post_run_scripts')
        self.data_dir = self.to_delete[1]
        # tmp files for tests
        self.tmpdir, self.filelist = setup_dir_with_tmp_files()
        # adjust server config for job to include above tmpdir
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd, Loader=yaml.SafeLoader)
            parsed['backup'][0]['paths'] = [self.tmpdir]
            self.job_name = parsed['backup'][0]['name']
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
        # start server
        self.base_url = "http://127.0.0.1:8080"
        if platform.system() == 'Windows':
            fh, self.inttestlog = tempfile.mkstemp(prefix="integration_test_log_")
            os.close(fh)
            # for some reason (I guess Python + Windows bug) output to stdout which is beyond some arbitrary length make
            # the test fail; ugly workaround is to send output to the logfile
            self.daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=self.base_url,
                                       extra_options="--logfile=" + self.inttestlog)
        else:
            self.daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=self.base_url)
        self.api_root = '/api/v1'

    def tearDown(self):
        self.daemon.kill()
        # remove config file and any tmp dirs required by config file statements
        for entry in self.to_delete:
            if os.path.exists(entry):
                if os.path.isdir(entry):
                    shutil.rmtree(entry)
                else:
                    os.remove(entry)
        if os.path.exists(self.tmpdir):
            shutil.rmtree(self.tmpdir)
        if platform.system() == 'Windows':
            if os.path.exists(self.inttestlog):
                os.remove(self.inttestlog)
        if os.path.exists(self.script_path):
            os.remove(self.script_path)
        if os.path.exists(self.script_result_path):
            os.remove(self.script_result_path)

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

    # starts a backup which has a pre_run script configured and tests that it runs as expected
    def test_backup_job_start_stop_scripts1(self):
        # fetch config
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/config'))
        # check if response can be JSON decoded
        response = r.json()
        # adjust first backup so it has the new script path
        job_index = 0
        found = False
        for index, value in enumerate(response['result']['backup']):
            if value['name'] == self.job_name:
                job_index = index
                found = True
        self.assertTrue(found, "Did not find any backup job having name \"{}\"".format(self.job_name))

        # adjust cfg contents
        response['result']['backup'][job_index]['pre_run_script'] = self.script_path
        response['result']['backup'][job_index]['target'][0]['ratelimit'] = "10"

        # send to server updated config
        logging.info("Adjusting service config via the API")
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          json=response['result'])
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                                 "changes are going to take effect")

        # start backup job
        logging.info("Starting backup for job: {}".format(self.job_name))

        # attempt to start backup
        req = {"name": self.job_name}
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

        logging.info("Checking if backup job is running")
        # fetch again list of jobs and check that status of job is now "running"
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        is_running = False
        job_id_matches = False
        found_job_id = ""
        for backup in response['result']:
            if backup['name'] == self.job_name and backup['state'] == 'running':
                is_running = True
                if backup['job_id'] == job_id:
                    job_id_matches = True
                else:
                    found_job_id = backup['job_id']
        self.assertTrue(is_running, "did not manage to find a running backup for job having name: '{}'. "
                                    "Response from server was: {}".format(self.job_name, r.text))
        self.assertTrue(job_id_matches, "While job named '{}' is running, the job id does not match. Expected to find"
                                        "job id '{}' but found instead '{}'. Full response is:"
                                        " {}".format(self.job_name, job_id, found_job_id, r.text))
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

        logging.info("Checking if current_operation == 'Running pre_run_script ...'")
        # now that the state is running, given that when a backup starts we have a 1 second sleep (hardcoded in server)
        #  now attempt to figure out when the pre run script started and ensure that
        #    'current_operation' == 'Running pre_run_script ...'
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            script_is_running = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'running':
                    if backup['stats_text']['current_operation'] == "Running pre_run_script {}".format(self.script_path):
                        script_is_running = True
                        break
            if script_is_running:
                break
            else:
                if counter > 50:
                    self.fail("Backup did not start running the pre_run_script after 5 seconds checking")
                time.sleep(0.1)
                counter += 1

        logging.info("Checking if current_operation == 'Examining and backing up'")
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            file_backup_started = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'running':
                    if backup['stats_text']['current_operation'] == "Examining and backing up":
                        file_backup_started = True
                        self.assertEqual(0, backup['stats_counters']["scripts_failed"], "At least pre/post run script"
                                                                                        " has failed")
                        self.assertEqual(1, backup['stats_counters']["scripts_ran"],
                                         "Status should report 1 script ran but it reports that {} scripts "
                                         "ran".format(backup['stats_counters']["scripts_ran"]))
                        self.assertEqual(1, backup['stats_counters']["scripts_num"],
                                         "Status should report 1 script exists but it reports that {} scripts "
                                         "exist".format(backup['stats_counters']["scripts_num"]))
                        break
                if backup['name'] == self.job_name and backup['state'] != 'running':
                    pprint(backup)
                    self.fail("Backup has exited prematurely the 'running' state due to unknown issue (see above "
                              "pretty print)")

            if file_backup_started:
                break
            if counter > 70:
                pprint(response['result'])
                self.fail(
                    "Backup did not complete running the pre_run_script and proceed to backup files after 7"
                    " seconds checking(see above pretty print for last job status)")
            time.sleep(0.1)
            counter += 1

        logging.info("Checking that the file produced by pre_run_script has the expected content")
        # Windows seems sometimes to be slow to release file locks , so let's try several times
        counter = 0
        while True:
            try:
                with open(self.script_result_path, "r") as fd:
                    file_content = fd.read()
                    break
            except PermissionError:
                if counter > 50:
                    self.fail("File lock not released in 5 seconds or the user running the test can't access the file")
                time.sleep(0.1)
                counter += 1
        self.assertEqual(job_id, file_content.rstrip(), "The file produced by the script should have as "
                                                        "content the job id, but it doesn't ")

        # attempt to stop backup using user which has the right privileges
        req = {"name": self.job_name}
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
            if backup['name'] == self.job_name and (backup['state'] == 'stopping' or backup['state'] == 'stopped'):
                is_stopping_or_stopped = True
        self.assertTrue(is_stopping_or_stopped, "did not manage to find a backup for job having name: '{}' and state "
                                                "one of 'stopping' or 'stopped'. Response from server "
                                                "was: {}".format(self.job_name, r.text))

    # starts a backup which has a post_run script configured and tests that it runs as expected
    # additionally after the backup finishes, open to the sqlite DB and asses the data matches expectations
    def test_backup_job_start_stop_scripts2(self):
        # fetch config
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/config'))
        # check if response can be JSON decoded
        response = r.json()
        # adjust first backup so it has the new script path
        job_index = 0
        found = False
        for index, value in enumerate(response['result']['backup']):
            if value['name'] == self.job_name:
                job_index = index
                found = True
        self.assertTrue(found, "Did not find any backup job having name \"{}\"".format(self.job_name))

        # adjust cfg contents
        response['result']['backup'][job_index]['post_run_script'] = self.script_path
        response['result']['backup'][job_index]['target'][0]['ratelimit'] = "100"
        sqlite_db_file = response['result']['data_dir'] + os.sep + self.job_name + ".sqlite"

        # send to server updated config
        logging.info("Adjusting service config via the API")
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          json=response['result'])
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                                 "changes are going to take effect")

        # start backup job
        logging.info("Starting backup for job: {}".format(self.job_name))

        # attempt to start backup
        req = {"name": self.job_name}
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

        logging.info("Checking if backup job is running")
        # fetch again list of jobs and check that status of job is now "running"
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        is_running = False
        job_id_matches = False
        found_job_id = ""
        for backup in response['result']:
            if backup['name'] == self.job_name and backup['state'] == 'running':
                is_running = True
                if backup['job_id'] == job_id:
                    job_id_matches = True
                else:
                    found_job_id = backup['job_id']
        self.assertTrue(is_running, "did not manage to find a running backup for job having name: '{}'. "
                                    "Response from server was: {}".format(self.job_name, r.text))
        self.assertTrue(job_id_matches, "While job named '{}' is running, the job id does not match. Expected to find"
                                        "job id '{}' but found instead '{}'. Full response is:"
                                        " {}".format(self.job_name, job_id, found_job_id, r.text))
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

        logging.info("Checking if current_operation == 'Examining and backing up'")
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            file_backup_started = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'running':
                    if backup['stats_text']['current_operation'] == "Examining and backing up":
                        file_backup_started = True
                        break
                if backup['name'] == self.job_name and backup['state'] != 'running':
                    pprint(backup)
                    self.fail("Backup has exited prematurely the 'running' state due to unknown issue (see above "
                              "pretty print)")

            if file_backup_started:
                break
            if counter > 70:
                pprint(response['result'])
                self.fail(
                    "Backup did not complete running the pre_run_script and proceed to backup files after 7"
                    " seconds checking(see above pretty print for last job status)")
            time.sleep(0.1)
            counter += 1

        logging.info("Checking if current_operation == 'Running post_run_script ...'")
        # now that the state is running, given that when a backup starts we have a 1 second sleep (hardcoded in server)
        #  now attempt to figure out when the pre run script started and ensure that
        #    'current_operation' == 'Running post_run_script ...'
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            script_is_running = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'running':
                    if backup['stats_text']['current_operation'] == "Running post_run_script {}".format(
                            self.script_path):
                        script_is_running = True
                        self.assertEqual(0, backup['stats_counters']["scripts_failed"], "At least pre/post run script"
                                                                                        " has failed")
                        self.assertEqual(1, backup['stats_counters']["scripts_ran"],
                                         "Status should report 1 script ran but it reports that {} scripts "
                                         "ran".format(backup['stats_counters']["scripts_ran"]))
                        self.assertEqual(1, backup['stats_counters']["scripts_num"],
                                         "Status should report 1 script exists but it reports that {} scripts "
                                         "exist".format(backup['stats_counters']["scripts_num"]))
                        break
            if script_is_running:
                break
            else:
                if counter > 100:
                    self.fail("Backup did not start running the post_run_script after 10 seconds checking")
                time.sleep(0.1)
                counter += 1

        logging.info("Checking if current_operation == 'stopping'")
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            script_is_stopping = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'stopping':
                    script_is_stopping = True
                    job_status_while_stopping = backup
                    break
            if script_is_stopping:
                break
            else:
                if counter > 60:
                    self.fail("Backup did not stop running after 6 seconds of checking")
                time.sleep(0.1)
                counter += 1

        logging.info("Checking that the file produced by pre_run_script has the expected content")
        # Windows seems sometimes to be slow to release file locks , so let's try several times
        counter = 0
        while True:
            try:
                with open(self.script_result_path, "r") as fd:
                    file_content = fd.read()
                    break
            except PermissionError:
                if counter > 50:
                    self.fail("File lock not released in 5 seconds or the user running the test can't access the file")
                time.sleep(0.1)
                counter += 1
        self.assertEqual(job_id, file_content.rstrip(), "The file produced by the script should have as "
                                                        "content the job id, but it doesn't ")

        logging.info("Checking if current_operation == 'stopped'")
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            script_is_stopping = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'stopped':
                    script_is_stopping = True
                    break
            if script_is_stopping:
                break
            else:
                if counter > 60:
                    logging.info(response['result'])
                    logging.info("#########")
                    _, stderr, stdout = self.daemon.stop(get_output=True)
                    logging.info("Daemon's stdout + stderr were:\n {}".format(stdout + stderr))
                    self.fail("Backup did not complete stopping after 6 seconds of checking (after it entered"
                              " 'stopping' state")
                time.sleep(0.1)
                counter += 1

        logging.info("Fetch job status report from the sqlite database and compare it with the job status we got "
                     "when checking if job=stopping")
        # connect to DB, fetch data
        conn = sqlite3.connect(sqlite_db_file)
        cur = conn.cursor()
        query_tuple = (job_id, self.job_name, "backup")
        cur.execute('SELECT * FROM jobs WHERE id = ? AND name = ? AND type = ? ', query_tuple)
        rows = cur.fetchall()
        self.assertEqual(1, len(rows), "Was expecting to find 1 matching row in the DB but instead "
                                       "found {}".format(len(rows)))
        job_details = rows[0]
        self.assertEqual(job_details[5], "finished", "Was expecting job status to be finished but instead it "
                                                     "is {}".format(job_details[5]))
        job_status_report = json.loads(job_details[6])
        # change status from stopping to finished so then we can compare the dicts
        # change status from stopping to finished so then we can compare the dicts
        job_status_while_stopping["state"] = "finished"
        # copy end time from status extract from DB as there is no way to know otherwise when it finished
        job_status_while_stopping["end_time"] = job_status_report["end_time"]
        # show the full diff in self.assertDictEqual
        self.maxDiff = None
        self.assertDictEqual(job_status_while_stopping, job_status_report)
        # close db connection
        conn.close()

    # starts a backup which has a post_run script configured and tests that it runs as expected despite the backup
    # being cancelled
    def test_backup_job_start_stop_scripts3(self):
        # fetch config
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/config'))
        # check if response can be JSON decoded
        response = r.json()
        # adjust first backup so it has the new script path
        job_index = 0
        found = False
        for index, value in enumerate(response['result']['backup']):
            if value['name'] == self.job_name:
                job_index = index
                found = True
        self.assertTrue(found, "Did not find any backup job having name \"{}\"".format(self.job_name))

        # adjust cfg contents
        response['result']['backup'][job_index]['post_run_script'] = self.script_path
        response['result']['backup'][job_index]['target'][0]['ratelimit'] = "10"

        # send to server updated config
        logging.info("Adjusting service config via the API")
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          json=response['result'])
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                                 "changes are going to take effect")

        # start backup job
        logging.info("Starting backup for job: {}".format(self.job_name))

        # attempt to start backup
        req = {"name": self.job_name}
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

        logging.info("Checking if backup job is running")
        # fetch again list of jobs and check that status of job is now "running"
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        is_running = False
        job_id_matches = False
        found_job_id = ""
        for backup in response['result']:
            if backup['name'] == self.job_name and backup['state'] == 'running':
                is_running = True
                if backup['job_id'] == job_id:
                    job_id_matches = True
                else:
                    found_job_id = backup['job_id']
        self.assertTrue(is_running, "did not manage to find a running backup for job having name: '{}'. "
                                    "Response from server was: {}".format(self.job_name, r.text))
        self.assertTrue(job_id_matches, "While job named '{}' is running, the job id does not match. Expected to find"
                                        "job id '{}' but found instead '{}'. Full response is:"
                                        " {}".format(self.job_name, job_id, found_job_id, r.text))
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

        logging.info("Checking if current_operation == 'Examining and backing up'")
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            file_backup_started = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'running':
                    if backup['stats_text']['current_operation'] == "Examining and backing up":
                        file_backup_started = True
                        break
                if backup['name'] == self.job_name and backup['state'] != 'running':
                    pprint(backup)
                    self.fail("Backup has exited prematurely the 'running' state due to unknown issue (see above "
                              "pretty print)")

            if file_backup_started:
                break
            if counter > 70:
                pprint(response['result'])
                self.fail(
                    "Backup did not complete running the pre_run_script and proceed to backup files after 7"
                    " seconds checking(see above pretty print for last job status)")
            time.sleep(0.1)
            counter += 1

        logging.info("Cancelling the backup")
        req = {"name": self.job_name}
        url = self.base_url + self.api_root + '/backup/stop'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("name", response['result'],
                      "For {} response['result'] is missing the 'name' key. Response was:"
                      " {}".format(url, r.text))
        self.assertIn("job_id", response['result'],
                      "For {} response['result'] is missing the 'job_id' key. Response "
                      "was: {}".format(url, r.text))

        logging.info("Checking if state == 'stopping'")
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            script_is_stopping = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'stopping':
                    script_is_stopping = True
                    break
            if script_is_stopping:
                break
            else:
                if counter > 60:
                    self.fail("Backup did not stop running after 6 seconds of checking")
                time.sleep(0.1)
                counter += 1

        logging.info("Checking if current_operation == 'Running post_run_script ...'")
        # now that the state is running, given that when a backup starts we have a 1 second sleep (hardcoded in server)
        #  now attempt to figure out when the pre run script started and ensure that
        #    'current_operation' == 'Running post_run_script ...'
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            script_is_running = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'stopping':
                    if backup['stats_text']['current_operation'] == "Running post_run_script {}".format(
                            self.script_path):
                        script_is_running = True
                        self.assertEqual(0, backup['stats_counters']["scripts_failed"], "At least pre/post run script"
                                                                                        " has failed")
                        self.assertEqual(1, backup['stats_counters']["scripts_ran"],
                                         "Status should report 1 script ran but it reports that {} scripts "
                                         "ran".format(backup['stats_counters']["scripts_ran"]))
                        self.assertEqual(1, backup['stats_counters']["scripts_num"],
                                         "Status should report 1 script exists but it reports that {} scripts "
                                         "exist".format(backup['stats_counters']["scripts_num"]))
                        break
            if script_is_running:
                break
            else:
                if counter > 100:
                    self.fail("Backup did not start running the post_run_script after 10 seconds checking")
                time.sleep(0.1)
                counter += 1

        logging.info("Checking if state == 'stopped'")
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is matching expectations
            url = self.base_url + self.api_root + '/backup/list'
            r = requests.get(url=url, auth=(self.username, self.password))
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            script_is_stopping = False
            for backup in response['result']:
                if backup['name'] == self.job_name and backup['state'] == 'stopped':
                    script_is_stopping = True
                    break
            if script_is_stopping:
                break
            else:
                if counter > 60:
                    self.fail("Backup did not stop running after 6 seconds of checking")
                time.sleep(0.1)
                counter += 1

        logging.info("Checking that the file produced by pre_run_script has the expected content")
        # Windows seems sometimes to be slow to release file locks , so let's try several times
        counter = 0
        while True:
            try:
                with open(self.script_result_path, "r") as fd:
                    file_content = fd.read()
                    break
            except PermissionError:
                if counter > 50:
                    self.fail("File lock not released in 5 seconds or the user running the test can't access the file")
                time.sleep(0.1)
                counter += 1
        self.assertEqual(job_id, file_content.rstrip(), "The file produced by the script should have as "
                                                        "content the job id, but it doesn't ")


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIBackupRunScripts)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
