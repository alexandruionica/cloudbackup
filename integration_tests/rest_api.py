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
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
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

        request_api_path('/config', method='GET')
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

    # test different scenarios regarding updating the configuration of the daemon using /config
    #  this is mainly a copy of the actions of the previous test
    def test_put_config(self):
        orig_md5 = get_md5_sum(self.config_file_path)
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/config'))
        # check if response can be JSON decoded
        response = r.json()
        # just send back the config we got
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          data=json.dumps(response['result']))
        # should have failed as we did not set content-type to be JSON
        self.assertEqual(r.status_code, 400, r.text)
        self.assertEqual(r.json()['code'], "bad content type")

        # repeat request but this time we use json= which does itself encoding to json and also sets content-type
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          json=response['result'])
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                              "changes are going to take effect")
        # check that md5 of file on disk dit NOT change
        current_md5 = get_md5_sum(self.config_file_path)
        self.assertEqual(orig_md5, current_md5)

        # repeat request, this time with a username + password which don't have access to update the configuratio
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username2, self.password2),
                          json=response['result'])
        self.assertEqual(r.status_code, 403, r.text)
        self.assertEqual(r.json()['code'], "access denied")

        # repeat request, this time we change the payload so it should succeed changing the config and we use the
        #  correct username + password
        payload = response['result']
        payload['https']['bind_address'] = "127.0.0.1:8444"
        r = requests.post(self.base_url + self.api_root + '/config', auth=(self.username, self.password),
                          json=payload)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                                 "changes are going to take effect")
        # fetch again config to validate that the changed config is shown in responses
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()['result']['https']['bind_address'], payload['https']['bind_address'])
        self.assertDictEqual(r.json()['result'], payload)
        # check that md5 of file on disk changed
        current_md5 = get_md5_sum(self.config_file_path)
        self.assertNotEqual(orig_md5, current_md5)

    # test different scenarios regarding updating the configuration of the daemon using /config/backup
    def test_put_config_backup(self):
        orig_md5 = get_md5_sum(self.config_file_path)
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/config'))
        # check if response can be JSON decoded
        response = r.json()
        # just send back the config we got for the 1st backup
        r = requests.post(self.base_url + self.api_root + '/config/backup', auth=(self.username, self.password),
                          data=json.dumps(response['result']['backup'][0]))
        # should have failed as we did not set content-type to be JSON
        self.assertEqual(r.status_code, 400, r.text)
        self.assertEqual(r.json()['code'], "bad content type")

        # repeat request but this time we use json= which does itself encoding to json and also sets content-type
        r = requests.post(self.base_url + self.api_root + '/config/backup', auth=(self.username, self.password),
                          json=response['result']['backup'][0])
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                              "changes are going to take effect")
        # check that md5 of file on disk dit NOT change
        current_md5 = get_md5_sum(self.config_file_path)
        self.assertEqual(orig_md5, current_md5)

        # repeat request, this time with a username + password which don't have access to update the configuratio
        r = requests.post(self.base_url + self.api_root + '/config/backup', auth=(self.username2, self.password2),
                          json=response['result'])
        self.assertEqual(r.status_code, 403, r.text)
        self.assertEqual(r.json()['code'], "access denied")

        # repeat request, this time we change the payload so it should succeed changing the config and we use the
        #  correct username + password
        payload = response['result']['backup'][0]
        payload['schedule'][0] = '06 11 * * *'
        r = requests.post(self.base_url + self.api_root + '/config/backup', auth=(self.username, self.password),
                          json=payload)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotEqual(r.json()['message'], "The supplied configuration matches the existing one so no actual "
                                                 "changes are going to take effect")
        # fetch again config to validate that the changed config is shown in responses
        r = requests.get(self.base_url + self.api_root + '/config', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()['result']['backup'][0]['schedule'][0], payload['schedule'][0])
        self.assertDictEqual(r.json()['result']['backup'][0], payload)
        # check that md5 of file on disk changed
        current_md5 = get_md5_sum(self.config_file_path)
        self.assertNotEqual(orig_md5, current_md5)

    # check list of backup jobs
    def test_get_backup_jobs_list(self):
        r = requests.get(self.base_url + self.api_root + '/backup/list', auth=(self.username, self.password))
        self.assertEqual(r.status_code, 200, "Expected status code 200 for GET "
                                             "{}".format(self.base_url + self.api_root + '/backup/list'))
        # check if response can be JSON decoded
        response = r.json()
        # check response has expected keys
        self.assertIn("code", response, "Response is missing the 'code' key ")
        self.assertIn("message", response, "Response is missing the 'message' key ")
        self.assertIn("result", response, "Response is missing the 'result' key ")
        self.assertGreaterEqual(2, len(response['result']), "'result' key should have at least 2 results contained")
        self.assertIn("name", response['result'][0], "response['result'][0] is missing the 'name' key ")
        self.assertIn("state", response['result'][0], "response['result'][0] is missing the 'state' key ")
        self.assertIn("start_time", response['result'][0], "response['result'][0] is missing the 'start_time' key ")
        self.assertIn("next_run", response['result'][0], "response['result'][0] is missing the 'next_run' key ")


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
