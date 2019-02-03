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


class TestRestAPIReportNotification2(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        self.extra_server_cfg = '''notification:
          email:
            - server: 127.0.0.1
              port: 25025
              to: "someone@foobar.com"
        '''
        self.complete_server_cfg = working_server_config_file_content + self.extra_server_cfg
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_rest_api_report_notification2_', config_file_content=self.complete_server_cfg)
        # tmp files for tests
        self.tmpdir, self.filelist = setup_dir_with_tmp_files()
        # adjust server config for job to include above tmpdir
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd)
            parsed['backup'][0]['paths'] = [self.tmpdir]
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
        # start SMTP server on Linux only as it doesn't work on other platforms
        if platform.system().lower() == 'linux':
            self.mock_server = MockSMTPServer("localhost", 25025)
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
        if platform.system().lower() == 'linux':
            self.mock_server.stopsmtpsrv()

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
        # unfortunately the mock SMTP server we use runs only on Linux so we can't run the tests on other platforms
        if platform.system().lower() != 'linux':
            logging.warn("SKIPPING SMTP related tests as they can't run on other platforms than Linux as the SMTP "
                         "server used is working only on Linux.")
            return
        r = requests.post(self.base_url + self.api_root + '/report/notification/test', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for POST "
                                             "{}".format(self.base_url + self.api_root + '/report/notification/test'))
        # check if response can be JSON decoded
        response = r.json()
        # check response has expected keys
        self.assertIn("code", response, "Response is missing the 'code' key. Response was: {}".format(r.text))
        self.assertIn("message", response, "Response is missing the 'message' key. Response was: {}".format(r.text))
        self.assertEqual(response["code"], "success")
        self.assertEqual(response["message"], "Test completed successfully")

        # verify that the message has been received
        self.assertEqual(self.mock_server.received_messages_count(), 1, "Was expecting exactly 1 email message to have "
                                                                        "been received")
        # verify that the message matches From: expectations
        hostname = socket.gethostname()
        matches, email_msg = self.mock_server.received_message_matching(".*From: cloudbackup@{}.*".format(hostname))
        self.assertTrue(matches, "email doesn't match From: expectations. What we received was: {}".format(email_msg))

        # verify that the message matches To: expectations
        matches, email_msg = self.mock_server.received_message_matching(".*To: someone@foobar.com.*")
        self.assertTrue(matches, "email doesn't match To: expectations. What we received was: {}".format(email_msg))

        # verify that the message matches Subject: expectations
        matches, email_msg = self.mock_server.received_message_matching(".*Subject: Notification test.*")
        self.assertTrue(matches, "email doesn't match Subject: expectations. What we received "
                                 "was: {}".format(email_msg))

        # verify that the message matches body(data) expectations
        matches, email_msg = self.mock_server.received_message_matching(".*Receiving this email proves that the backup"
                                                                        " server's SMTP\(email\) settings are "
                                                                        "correct.*")
        self.assertTrue(matches, "email doesn't match body(data) expectations. What we received "
                                 "was: {}".format(email_msg))

    # start a backup, wait for it to end and then check notification - should work
    def test_notification_test2(self):
        # unfortunately the mock SMTP server we use runs only on Linux so we can't run the tests on other platforms
        if platform.system().lower() != 'linux':
            logging.warn("SKIPPING SMTP related tests as they can't run on other platforms than Linux as the SMTP "
                         "server used is working only on Linux.")
            return
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
        job_id = response['result']['job_id']

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
        # verify that the message has been received
        self.assertEqual(self.mock_server.received_messages_count(), 1, "Was expecting exactly 1 email message to have "
                                                                        "been received")
        # verify that the message matches From: expectations
        hostname = socket.gethostname()
        try:
            matches, email_msg = self.mock_server.received_message_matching(".*From: cloudbackup@{}.*".format(hostname))
        except UnicodeDecodeError:
            self.fail("1.Did not manage to fully decode the UTF-8 body of the email. Most likely this means that one of"
                      " the fields you are searching for was not found while scanning the text part of the email and"
                      " then when examining the HTML bit the UTF decoded error happened. The received email"
                      " was: {}".format(self.mock_server.received_messages[0]))
        self.assertTrue(matches, "email doesn't match From: expectations. What we received was: {}".format(email_msg))

        # verify that the message matches To: expectations
        try:
            matches, email_msg = self.mock_server.received_message_matching(".*To: someone@foobar.com.*")
        except UnicodeDecodeError:
            self.fail("2.Did not manage to fully decode the UTF-8 body of the email. Most likely this means that one of"
                      " the fields you are searching for was not found while scanning the text part of the email and"
                      " then when examining the HTML bit the UTF decoded error happened. The received email"
                      " was: {}".format(self.mock_server.received_messages[0]))
        self.assertTrue(matches, "email doesn't match To: expectations. What we received was: {}".format(email_msg))

        # verify that the message matches Subject: expectations
        try:
            matches, email_msg = self.mock_server.received_message_matching(".*Subject: backup "
                                                                            "job \"{}\" has finished.*".format(job_name))
        except UnicodeDecodeError:
            self.fail("3.Did not manage to fully decode the UTF-8 body of the email. Most likely this means that one of"
                      " the fields you are searching for was not found while scanning the text part of the email and"
                      " then when examining the HTML bit the UTF decoded error happened. The received email"
                      " was: {}".format(self.mock_server.received_messages[0]))
        self.assertTrue(matches, "email doesn't match Subject: expectations. What we received "
                                 "was: {}".format(email_msg))

        # verify that the message matches body(data) expectations
        try:
            matches, email_msg = self.mock_server.received_message_matching(".*backup job \"{}\" having id {} has "
                                                                            "finished.*".format(job_name, job_id))
        except UnicodeDecodeError:
            self.fail("4.Did not manage to fully decode the UTF-8 body of the email. Most likely this means that one of"
                      " the fields you are searching for was not found while scanning the text part of the email and"
                      " then when examining the HTML bit the UTF decoded error happened. The received email"
                      " was: {}".format(self.mock_server.received_messages[0]))
        self.assertTrue(matches, "email doesn't match body(data) expectations. What we received "
                                 "was: {}".format(email_msg))


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIReportNotification2)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
