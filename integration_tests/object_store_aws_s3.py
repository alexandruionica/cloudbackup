#!/usr/bin/env python
#
# CLI Tests which require a server to be running as a prerequisite of the test
#
#
import argparse
import shutil
import sys
import unittest
import yaml
from common import *


class TestObjectStoreAwsS3(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_object_store_aws_s3_')
        # client - config file
        tmphandle, self.client_config_file_path = tempfile.mkstemp(suffix='_integration_tests_client_config_file.yaml')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(working_client_config_file_content)
        tmpfile.close()
        # tmp files for tests
        self.tmpdir, self.filelist = setup_dir_with_tmp_files()
        # adjust server config for job to include above tmpdir
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd)
            parsed['backup'][0]['paths'] = [self.tmpdir]
            parsed['backup'][0]['exclusions'] = [""]
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
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

    # ./cloudbackup client backup target test first_backup -c client_config.yaml should fail due to
    # incorrect credentials for S3
    def test_cmd_client_backup_target_test1(self):
        bucket, _, _, _ = get_s3_config_from_env()
        self.assertIsNotNone(bucket, "Environment variable CLD_S3_BUCKET is not set")
        tested_job_name = "first_backup"
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
            if value['name'] == tested_job_name:
                job_index = index
                found = True
        self.assertTrue(found, "Did not find any backup job having name \"{}\"".format(tested_job_name))

        # adjust cfg contents
        response['result']['backup'][job_index]['target'][0]['type'] = "aws_s3"
        response['result']['backup'][job_index]['target'][0]['bucket'] = bucket
        response['result']['backup'][job_index]['target'][0]['parameters'] = [
            {"name": "AWS_ACCESS_KEY_ID",
             "value": "bad_key_id"},
            {"name": "AWS_SECRET_ACCESS_KEY",
             "value": "bad_secret"},
            {"name": "region",
             "value": "us-east-1"}
        ]

        # send to server updated config
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          json=response['result'])
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                                 "changes are going to take effect")

        result = run_shell_cmd(self.cmd + " client backup target test " + tested_job_name +
                               " -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))

    def test_full_backup(self):
        bucket, s3_region, aws_key, aws_secret = get_s3_config_from_env()
        self.assertIsNotNone(bucket, "Environment variable CLD_S3_BUCKET is not set")
        job_name = "first_backup"
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
            if value['name'] == job_name:
                job_index = index
                found = True
        self.assertTrue(found, "Did not find any backup job having name \"{}\"".format(job_name))

        # adjust cfg contents
        response['result']['backup'][job_index]['target'][0]['type'] = "aws_s3"
        response['result']['backup'][job_index]['target'][0]['bucket'] = bucket
        # the cleanup script (at the end of the integration tests will take care of cleaning up the bucket
        if self.id().startswith("__main__."):
            prefix = "tests/" + platform.system().lower() + "/" + self.id()[9:]
        else:
            prefix = "tests/" + platform.system().lower() + "/" + self.id()
        response['result']['backup'][job_index]['target'][0]['prefix'] = prefix
        response['result']['backup'][job_index]['target'][0]['parameters'] = [
            {"name": "region",
             "value": s3_region}
        ]
        if aws_key and aws_secret:
            response['result']['backup'][job_index]['target'][0]['parameters'].append(
                {"name": "AWS_ACCESS_KEY_ID",
                 "value": aws_key}
            )
            response['result']['backup'][job_index]['target'][0]['parameters'].append(
                {"name": "AWS_SECRET_ACCESS_KEY",
                 "value": aws_secret}
            )

        # send to server updated config
        logging.info("Adjusting service config via the API")
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          json=response['result'])
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                                 "changes are going to take effect")

        logging.info("Testing object store credentials and configuration are valid")
        result = run_shell_cmd(self.cmd + " client backup target test " +
                               job_name + " -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))

        # attempt to start backup with user having correct privileges
        logging.info("Starting backup with destination S3: {}/{}".format(bucket, prefix))
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
                if counter > 100:
                    self.fail("Backup did not finish running in 10 seconds")
                time.sleep(0.1)
                counter += 1

        # TODO - get report of backup job and check that there were no errors and that the expected number of files
        # got backed up


def get_args():
    """ Get arguments from CLI """

    parser = argparse.ArgumentParser(description='Script which performs integration tests')
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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestObjectStoreAwsS3)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
