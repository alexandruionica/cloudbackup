#!/usr/bin/env python
#
# API Tests which require a server to be running as a prerequisite of the test
#
#
import argparse
import shutil
import sys
import tempfile
import unittest
import requests
import yaml
from common import *


class TestRestAPIReportBackupList1(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_report_backup_list_')
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
        # start server
        self.base_url = "http://127.0.0.1:8080"
        # for some reason output to stdout which is beyond some arbitrary length make
        # the test fail (Windows is more sensitive than Linux); workaround is to send output to the logfile
        _, self.inttestlog = tempfile.mkstemp(prefix="integration_test_log_")
        if verbose:
            self.daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=self.base_url,
                                       extra_options="-d --logfile=" + self.inttestlog)
        else:
            self.daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=self.base_url,
                                       extra_options="--logfile=" + self.inttestlog)
            self.to_delete.append(self.inttestlog)
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
        if os.path.exists(self.client_config_file_path):
            os.remove(self.client_config_file_path)
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

    # get list of reports for this job - should be 0
    def AAtest_report_backup_list_test1(self):
        # fetch list of jobs and start the first one
        url = self.base_url + self.api_root + '/backup/list'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        job_name = response['result'][0]['name']
        logging.info("Getting report list for backup for job: {}".format(job_name))

        req = {"name": job_name}
        # attempt to get listing
        url = self.base_url + self.api_root + '/report/backup/list'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("next", response, "For {} response is missing the 'next' key. Response "
                                        "was: {}".format(url, r.text))
        self.assertEqual("", response['next'], "For {} response['next'] is not empty, as expected. "
                                               "Response was: {}".format(url, r.text))
        self.assertEqual(0, len(response['result']), "For {} response['result'] is not an empty array as expected. "
                                                     "Response was: {}".format(url, r.text))

    # get list of reports for this job - should be 1
    def test_report_backup_list_test2(self):
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
        self.assertIn("name", response['result'],
                      "For {} response['result'] is missing the 'name' key. Response was:"
                      " {}".format(url, r.text))
        self.assertIn("job_id", response['result'],
                      "For {} response['result'] is missing the 'job_id' key. Response "
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

        logging.info("Fetching a new report listing to see it's possible while the backup is running and that it "
                     "doesn't have extra results")
        req = {"name": job_name}
        # attempt to get listing
        url = self.base_url + self.api_root + '/report/backup/list'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("next", response, "For {} response is missing the 'next' key. Response "
                                        "was: {}".format(url, r.text))
        self.assertEqual("", response['next'], "For {} response['next'] is not empty, as expected. "
                                               "Response was: {}".format(url, r.text))
        self.assertEqual(0, len(response['result']), "For {} response['result'] is not an empty array as expected. "
                                                     "Response was: {}".format(url, r.text))

        # wait for backup job to complete
        logging.info("Waiting for the backup job to complete")
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
                if counter > 200:
                    self.fail("Backup did not finish running in 20 seconds")
                time.sleep(0.1)
                counter += 1

        logging.info("Getting report list for backup for job: {}".format(job_name))

        req = {"name": job_name}
        # attempt to get listing
        url = self.base_url + self.api_root + '/report/backup/list'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("next", response, "For {} response is missing the 'next' key. Response "
                                        "was: {}".format(url, r.text))
        self.assertEqual("", response['next'], "For {} response['next'] is not empty, as expected. "
                                               "Response was: {}".format(url, r.text))
        self.assertEqual(1, len(response['result']), "For {} response['result'] is not an empty array as expected. "
                                                     "Response was: {}".format(url, r.text))
        for entry in response['result']:
            self.assertEqual(entry["job_id"], job_id, "Job id {} in report list doesn't match job id {} returned "
                                                      "when the backup was started".format(entry["job_id"], job_id))

        logging.info("Starting again backup for job: {}".format(job_name))
        # attempt to start backup with user having correct privileges
        req = {"name": job_name}
        url = self.base_url + self.api_root + '/backup/start'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("name", response['result'],
                      "For {} response['result'] is missing the 'name' key. Response was:"
                      " {}".format(url, r.text))
        self.assertIn("job_id", response['result'],
                      "For {} response['result'] is missing the 'job_id' key. Response "
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

        logging.info("Fetching a new report listing to see it's possible while the backup is running and that it "
                     "doesn't have more than one results")
        req = {"name": job_name}
        # attempt to get listing
        url = self.base_url + self.api_root + '/report/backup/list'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("next", response, "For {} response is missing the 'next' key. Response "
                                        "was: {}".format(url, r.text))
        self.assertEqual("", response['next'], "For {} response['next'] is not empty, as expected. "
                                               "Response was: {}".format(url, r.text))
        self.assertEqual(1, len(response['result']), "For {} response['result'] is not an array with 1 element as "
                                                     "expected. Response was: {}".format(url, r.text))

        # wait for backup job to complete
        logging.info("Waiting for the new backup job to complete")
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
                if counter > 200:
                    self.fail("Backup did not finish running in 20 seconds")
                time.sleep(0.1)
                counter += 1

        logging.info("Getting report list for backup for job: {}".format(job_name))

        req = {"name": job_name}
        # attempt to get listing
        url = self.base_url + self.api_root + '/report/backup/list'
        r = requests.post(url=url, auth=(self.username, self.password), json=req)
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("next", response, "For {} response is missing the 'next' key. Response "
                                        "was: {}".format(url, r.text))
        self.assertEqual("", response['next'], "For {} response['next'] is not empty, as expected. "
                                               "Response was: {}".format(url, r.text))
        self.assertEqual(2, len(response['result']), "For {} response['result'] is not an array with 2 elements as "
                                                     "expected. Response was: {}".format(url, r.text))
        found_match = False
        for entry in response['result']:
            if entry["job_id"] == job_id:
                found_match = True
        self.assertTrue(found_match, "Job id {} returned when the backup was started does not match any job id from the"
                                     " list of finished backup jobs".format(job_id))

        #  check pagination works
        logging.info("Getting report list for backup for job: {} with pagination set to 1".format(job_name))
        counter = 0

        found_match = False
        next_set = "to_be_filled___63b17aca-0dca-11ea-b159-afd5b8424c69"
        while next_set:
            counter += 1
            logging.info("Commencing request number {}".format(counter))
            if next_set == "to_be_filled___63b17aca-0dca-11ea-b159-afd5b8424c69":
                req = {"name": job_name,
                       "max_results": 1,
                       }
            else:
                req = {"name": job_name,
                       "max_results": 1,
                       "next": next_set,
                       }

            # attempt to get listing
            url = self.base_url + self.api_root + '/report/backup/list'
            r = requests.post(url=url, auth=(self.username, self.password), json=req)
            self.assertEqual(r.status_code, 200, url + " " + r.text)
            response = self.ValidatedAndDecodeResponse(r, url)
            # check response has expected keys
            self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                              " {}".format(url, r.text))
            self.assertIn("next", response, "For {} response is missing the 'next' key. Response "
                                            "was: {}".format(url, r.text))
            for entry in response['result']:
                if entry["job_id"] == job_id:
                    found_match = True
            next_set = response['next']
            if counter < 3:
                self.assertNotEqual("", response['next'], "For {} response['next'] is empty, but it was expected to be "
                                                          "non empty. Response was: {}".format(url, r.text))
                self.assertEqual(1, len(response['result']), "For {} response['result'] is not an array with 1 elemen"
                                                             "ts as expected. Response was: {}".format(url, r.text))
            else:
                self.assertEqual("", response['next'], "For {} response['next'] is not empty, as expected. "
                                                       "Response was: {}".format(url, r.text))
                self.assertEqual(0, len(response['result']), "For {} response['result'] is not an array with 0 elemen"
                                                             "ts as expected. Response was: {}".format(url, r.text))

        self.assertTrue(found_match, "Job id {} returned when the backup was started does not match any job id from the"
                                     " list of finished backup jobs".format(job_id))


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
    global verbose
    arguments = get_args()
    cmd_default = arguments.cmd
    if arguments.verbose:
        verbosity = 2
        verbose = True
    else:
        verbosity = 1

    logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.WARNING)

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIReportBackupList1)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
