#!/usr/bin/env python
import argparse
import bcrypt
import logging
import json
import os
import re
import requests
import sys
import tempfile
import unittest
import yaml
from common import *
from pprint import pprint


class TestRestAPI(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        tmphandle, self.config_file_path = tempfile.mkstemp(suffix='_integration_tests_rest_api.yaml')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(working_config_file_content)
        tmpfile.close()
        self.base_url = "http://127.0.0.1:8080"
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.daemon = BackupDaemon(config_path=self.config_file_path, base_url=self.base_url)
        self.api_root = '/api/v1'

    def tearDown(self):
        if os.path.exists(self.config_file_path):
            os.remove(self.config_file_path)
        self.daemon.kill()

    # this isn't actually part of the API but it's a simple enough test
    def test_root(self):
        r = requests.get(self.base_url)
        self.assertEqual(r.status_code, 200, "expected http response code 200 but got {}".format(r.status_code))

    # test that a list of endpoints expects authentication and that providing invalid credentials doesn't grant access
    def test_unauthenticted(self):
        def request_api_path(path, method='GET'):
            if method == 'GET':
                r = requests.get(self.base_url + self.api_root + path)
            elif method == 'POST':
                r = requests.post(self.base_url + self.api_root + path)
            else:
                self.fail('Requested HTTP method is {} but only GET and POST'.format(method))
            self.assertEqual(r.status_code, 401, "Expected to receive an authentication request for {} to {} but "
                                                 "instead got HTTP status code "
                                                 "{}".format(method, self.base_url + self.api_root + path,
                                                             r.status_code))
            # check if providing incorrect credentials grants access
            if method == 'GET':
                r = requests.get(self.base_url + self.api_root + path, auth=('badUsername', 'badPassword'))
            elif method == 'POST':
                r = requests.post(self.base_url + self.api_root + path, auth=('badUsername', 'badPassword'))
            self.assertEqual(r.status_code, 401, "Expected to receive Unauthorized response for {} to {} while using"
                                                 " incorrect credentials but instead got HTTP status code "
                                                 "{}".format(method, self.base_url + self.api_root + path,
                                                             r.status_code))

        request_api_path('/config')
        request_api_path('/config/', method='POST')
        request_api_path('/config/backup', method='POST')

    def test_get_config(self):
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/config'))
        # check if response can be JSON decoded
        r.json()

    # check if config response contains any passwords which were not obfuscated
    def test_get_config_obfuscated_passwords(self):
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/config'))
        # check if response can be JSON decoded
        response = r.json()
        # reencode response as JSON, this time use indenting so each element is on a different line. This makes it
        #  rather easy to test if any line containing the "pass" keyword has an unobfuscated password
        response_re_encoded = json.dumps(response, indent=4)
        line_num = 0
        for line in response_re_encoded.split('\n'):
            if 'pass' in line.lower():
                pass_field = line.split(':')[1]
                password = pass_field.split('"')[1]
                re_result = re.match('^(\*+)|()$', password)
                self.assertTrue(re_result, "output from GET {} has on line {} a password which doesn't seem to "
                                           "be obfuscated:\n{}".format(self.base_url + self.api_root + '/config',
                                                                       line_num, line))
            line_num += 1
        self.assertGreater(line_num, 90, "output from GET {} to be at least 90 lines "
                                         "long".format(self.base_url + self.api_root + '/config'))


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPI)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
