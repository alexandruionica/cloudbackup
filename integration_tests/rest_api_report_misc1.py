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
import os
import requests
import yaml
from common import *


class TestRestAPIReportMisc1(unittest.TestCase):
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
        file_descriptor, self.inttestlog = tempfile.mkstemp(prefix="integration_test_log_")
        os.close(file_descriptor)
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

    def test_report_version_test1(self):
        # attempt to get version reponse
        url = self.base_url + self.api_root + '/report/version'
        r = requests.get(url=url, auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, url + " " + r.text)
        response = self.ValidatedAndDecodeResponse(r, url)
        # check response has expected keys
        self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                          " {}".format(url, r.text))
        self.assertIn("AwsSdk", response['result'], "For {} response['result'] is missing the 'AwsSdk' key. "
                                                    "Response was: {}".format(url, r.text))
        self.assertNotEqual("", response['result']['AwsSdk'], "For {} response['result']['AwsSdk'] is empty, maybe the "
                                                              "script setting the versions has not ran. "
                                                              "Response was: {}".format(url, r.text))

        self.assertIn("GcpStorageSdk", response['result'], "For {} response['result'] is missing the 'GcpStorageSdk' "
                                                           "key. Response was: {}".format(url, r.text))
        self.assertNotEqual("", response['result']['GcpStorageSdk'], "For {} response['result']['GcpStorageSdk'] is "
                                                                     "empty, maybe the script setting the versions has "
                                                                     "not ran. Response was: {}".format(url, r.text))

        self.assertIn("AzureBlobStorageSdk", response['result'], "For {} response['result'] is missing the '"
                                                                 "AzureBlobStorageSdk' key. "
                                                                 "Response was: {}".format(url, r.text))
        self.assertNotEqual("", response['result']['AzureBlobStorageSdk'], "For {} "
                                                                           "response['result']['AzureBlobStorageSdk'] "
                                                                           "is empty, maybe the script setting the "
                                                                           "versions has not ran. "
                                                                           "Response was: {}".format(url, r.text))

        self.assertIn("CloudBackup", response['result'], "For {} response['result'] is missing the 'CloudBackup' "
                                                         "key. Response was: {}".format(url, r.text))
        self.assertNotEqual("", response['result']['CloudBackup'], "For {} response['result']['CloudBackup'] is "
                                                                   "empty, maybe the script setting the versions has "
                                                                   "not ran. Response was: {}".format(url, r.text))

        self.assertIn("BuildDate", response['result'], "For {} response['result'] is missing the 'BuildDate' "
                                                       "key. Response was: {}".format(url, r.text))
        self.assertNotEqual("", response['result']['BuildDate'], "For {} response['result']['BuildDate'] is "
                                                                 "empty, maybe the script setting the versions has "
                                                                 "not ran. Response was: {}".format(url, r.text))

        self.assertIn("OS", response['result'], "For {} response['result'] is missing the 'OS' "
                                                "key. Response was: {}".format(url, r.text))
        self.assertIn("Arch", response['result'], "For {} response['result'] is missing the 'Arch' "
                                                  "key. Response was: {}".format(url, r.text))
        self.assertIn("Runtime", response['result'], "For {} response['result'] is missing the 'Runtime' "
                                                     "key. Response was: {}".format(url, r.text))


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPIReportMisc1)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
