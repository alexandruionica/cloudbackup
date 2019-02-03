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


class TestRestAPIReportNotification1(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_rest_api_report_notification1_')
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
        # for some reason the below fails despite the cloudbackup.exe process supposed to be killed and the above
        # succeeding, for now just abandoning this as a non issue and leaving log files behind
        # if platform.system() == 'Windows':
        #     if os.path.exists(self.inttestlog):
        #         os.remove(self.inttestlog)

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

    # start a test notification - should fail as we don't have the config file entries for any
    def test_notification_test1(self):
        # unfortunately the mock SMTP server we use runs only on Linux so we can't run the tests on other platforms
        if platform.system().lower() != 'linux':
            logging.warn("SKIPPING SMTP related tests as they can't run on other platforms than Linux as the SMTP "
                         "server used is working only on Linux.")
            return
        r = requests.post(self.base_url + self.api_root + '/report/notification/test', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 500, "Expected status code 500 for POST "
                                             "{}".format(self.base_url + self.api_root + '/report/notification/test'))
        # check if response can be JSON decoded
        response = r.json()
        # check response has expected keys
        self.assertIn("code", response, "Response is missing the 'code' key. Response was: {}".format(r.text))
        self.assertIn("message", response, "Response is missing the 'message' key. Response was: {}".format(r.text))
        self.assertEqual(response["code"], "internal server error")
        self.assertEqual(response["message"], "Notification test can not be run as there are no notification entries in"
                                              " the server's configuration file")


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIReportNotification1)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
