#!/usr/bin/env python
import argparse
import bcrypt
import logging
import os
import re
import requests
import sys
import shutil
import tempfile
import unittest
import yaml
from common import *
from pprint import pprint
from swagger_tester import swagger_test


class TestRestAPISwagger(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_rest_api_swagger')
        self.base_url = "http://127.0.0.1:8080"
        self.daemon = BackupDaemon(config_path=self.config_file_path, base_url=self.base_url)

    def tearDown(self):
        self.daemon.kill()
        # remove config file and any tmp dirs required by config file statements
        for entry in self.to_delete:
            if os.path.exists(entry):
                if os.path.isdir(entry):
                    shutil.rmtree(entry)
                else:
                    os.remove(entry)

    def test_swagger_unauthorized(self):
        authorize_error = {
            'get': {
                '/api/v1/report/version': [401]
            }
        }
        swagger_test(app_url=self.base_url, authorize_error=authorize_error)
        stopped, _, _ = self.daemon.stop()
        self.assertTrue(stopped, "Backup daemon already stopped. Something must have gone wrong")

    # # bug in swagger-parser library prevents correct parsing
    # def test_swagger_authorized(self):
    #     # the below basic auth is for user = 'testuser1' ; password = 'HV}H/y?<9$]Z5N4N' ; should be reproduced by doing
    #     #      import requests.auth
    #     #      requests.auth._basic_auth_str(user, password)
    #     extra_headers = {
    #         "Authorization": 'Basic dGVzdHVzZXIxOkhWfUgveT88OSRdWjVONE4='
    #     }
    #     swagger_test(app_url=self.base_url, extra_headers=extra_headers)
    #     stopped, _, _ = self.daemon.stop()
    #     self.assertTrue(stopped, "Backup daemon already stopped. Something must have gone wrong")


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestRestAPISwagger)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
