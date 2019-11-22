#!/usr/bin/env python
#
# API Tests which require a server to be running as a prerequisite of the test
#
#
import argparse
import shutil
import os
import stat
import sys
import tempfile
import unittest
import requests
import yaml
from common import *

UNIX_SCRIPT = """#!/bin/sh
# arguments are supposed to be JobType, JobName, JobId, JobState, JobError, reportFile
if [ $# -ne 6 ]; then
    echo "expected 6 arguments but got $#"
    echo -n "received arguments were: "
    for var in "$@"; do
        echo -n "\"$var\" "
    done
    echo ""
    exit 1
fi
FOUND=0
case $1 in
    backup | restore | purge)
        FOUND=1
        ;;
esac
if [ $FOUND -ne 1 ]; then
    echo "First argument must be one of  backup | restore | purge  but it was: $1"
    exit 2
fi

if [ ! -f "$6" ]; then
    echo "The sixth argument is supposed to be a regular file but in this case it isn't. The argument is: $6"
    exit 3
fi"""

WINDOWS_SCRIPT = """@echo off
set found=0
set argC=0
for %%x in (%*) do Set /A argC+=1

IF /I "%argC%" NEQ "6" (
    echo expected 6 arguments but got %argC%
    echo #### Received arguments where - one per line:
    for %%x in (%*) do echo %%x
    echo #### End of received arguments
    exit 1
)

REM test input matches expectation
IF "%1" == "backup"  GOTO :cond
IF "%1" == "restore" GOTO :cond
IF "%1" == "purge"   GOTO :cond
GOTO :skip
:cond
set found=1
:skip

IF /I "%found%" NEQ "1" (
    echo First argument must be one of  backup, restore, purge  but it was: %1
    exit 2
)

IF NOT EXIST "%6" (
    echo The sixth argument is supposed to be a regular file but in this case it isn't. The argument is: %6
    exit 3
)

echo end
@echo on"""


class TestRestAPIReportNotification5(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # setup notification script
        if platform.system().lower() == 'windows':
            self.script = WINDOWS_SCRIPT
            script_suffix = "_integration_tests_rest_api_report_notification5_notification_script.bat"
        else:
            self.script = UNIX_SCRIPT
            script_suffix = "_integration_tests_rest_api_report_notification5_notification_script.sh"
        tmphandle, self.script_path = tempfile.mkstemp(suffix=script_suffix)
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(self.script)
        tmpfile.close()
        # make script executable
        st = os.stat(self.script_path)
        os.chmod(self.script_path, st.st_mode | stat.S_IEXEC)
        # setup server config file
        self.extra_server_cfg = """notification:
    script:
      - path: {}
        type:
          - started
          - finished
          - failed
          - cancelled
          - crashed""".format(self.script_path)
        self.complete_server_cfg = working_server_config_file_content + self.extra_server_cfg
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_rest_api_report_notification5_', config_file_content=self.complete_server_cfg)
        # tmp files for tests
        self.tmpdir, self.filelist = setup_dir_with_tmp_files()
        # adjust server config for job to include above tmpdir
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd, Loader=yaml.SafeLoader)
            parsed['backup'][0]['paths'] = [self.tmpdir]
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
        self.to_delete.append(self.script_path)
        # start server
        self.base_url = "http://127.0.0.1:8080"
        if platform.system() == 'Windows':
            _, self.inttestlog = tempfile.mkstemp(prefix="integration_test_log_")
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
        # for some reason the below fails despite the cloudbackup.exe process supposed to be killed and the above
        # succeeding, for now just abandoning this as a non issue and leaving log files behind
        # if platform.system() == 'Windows':
        #     if os.path.exists(self.inttestlog):
        #         os.remove(self.inttestlog)
        if os.path.exists(self.script_path):
            os.remove(self.script_path)

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

    # start a test notification - should work
    def test_notification_test1(self):
        r = requests.post(self.base_url + self.api_root + '/report/notification/test', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for POST {}. Response body was: "
                                             "{}".format(self.base_url + self.api_root + '/report/notification/test',
                                                         r.text))
        # check if response can be JSON decoded
        response = r.json()
        # check response has expected keys
        self.assertIn("code", response, "Response is missing the 'code' key. Response was: {}".format(r.text))
        self.assertIn("message", response, "Response is missing the 'message' key. Response was: {}".format(r.text))
        self.assertEqual(response["code"], "success")
        self.assertIn("Test completed successfully", response["message"])

    # start a backup, wait for it to end and then check notification - should work
    def test_notification_test2(self):
        # fetch list of jobs and start the first one
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        job_name = response['result'][0]['name']
        logging.info("Starting backup for job: {}".format(job_name))

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
        counter = 0
        while True:
            # fetch again list of jobs and check that status of job, until it is no longer running
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
            else:
                if counter > 50:
                    self.fail("Backup did not finish running in 5 seconds")
                time.sleep(0.1)
                counter += 1


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIReportNotification5)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
