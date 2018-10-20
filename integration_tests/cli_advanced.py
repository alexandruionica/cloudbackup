#!/usr/bin/env python
#
# CLI Tests which require a server to be running as a prerequisite of the test
#
#
import argparse
import bcrypt
import json
import logging
import os
import re
import sys
import shutil
import tempfile
import unittest
import yaml
from common import *
from pprint import pprint


class TestCliAdvanced(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_server_config_file')
        # client - config file
        tmphandle, self.client_config_file_path = tempfile.mkstemp(suffix='_integration_tests_client_config_file.yaml')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(working_client_config_file_content)
        tmpfile.close()
        # start server
        self.base_url = "http://127.0.0.1:8080"
        self.daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=self.base_url)

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

    # ./cloudbackup client backup list -c client_config.yaml returns 3 lines
    def test_cmd_client_backup_list1(self):
        result = run_shell_cmd(self.cmd + " client backup list -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # output is at least 3 lines long
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            line_num += 1
        self.assertGreater(line_num, 3, "Expected output from {} to be at least 3 lines long. Command output object: "
                                        "{}".format(cmd_default, result))

    # ./cloudbackup client backup list -c client_config.yaml --json returns 2 jobs which match the names we expect
    def test_cmd_client_backup_list2(self):
        result = run_shell_cmd(self.cmd + " client backup list -c " + self.client_config_file_path + " --json")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # output is JSON
        decoded = json.loads(result['result'].stdout.decode("utf-8"))
        # check we got 2 elements as expected
        self.assertEqual(len(decoded['result']), 2, "result section of respons was expected to have 2 elements but "
                                                    "instead is has {}. Command output object:"
                                                    " {}".format(len(decoded['result']), result))
        # check elements names match expectation
        found_first_backup = False
        found_second_backup = False
        for backup_job in decoded['result']:
            if backup_job['name'] == 'first_backup':
                found_first_backup = True
            elif backup_job['name'] == 'second_backup':
                found_second_backup = True
        self.assertTrue(found_first_backup, "'first_backup' was not found in list of backup jobs. Command output "
                                            "object: {}".format(result))
        self.assertTrue(found_second_backup, "'second_backup' was not found in list of backup jobs. Command output"
                                             " object: {}".format(result))
        # check all back jobs have state "stopped"
        for backup_job in decoded['result']:
            self.assertEqual(backup_job['state'], 'stopped', "At least one backup job does not have state='stopped'."
                                                             " Command output object: {}".format(result))

    # ./cloudbackup client backup list -c client_config.yaml returns exit code 1 when server is offline
    def test_cmd_client_backup_list3(self):
        self.daemon.kill()
        result = run_shell_cmd(self.cmd + " client backup list -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup client backup list -c client_config.yaml -u INVALID_USER returns exit code 1 and expected error msg
    def test_cmd_client_backup_list4(self):
        result = run_shell_cmd(self.cmd + " client backup list -c " + self.client_config_file_path + " -u INVALID_USER")
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))
        found_expected_error = False
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            if 'Invalid username or password'.lower() in line.lower():
                found_expected_error = True
        self.assertTrue(found_expected_error)

    # ./cloudbackup client backup start first_backup -c client_config.yaml works
    def test_cmd_client_backup_start_stop(self):
        result = run_shell_cmd(self.cmd + " client backup start first_backup -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # validate backup did start
        result = run_shell_cmd(self.cmd + " client backup list -c " + self.client_config_file_path + " --json")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # output is JSON
        decoded = json.loads(result['result'].stdout.decode("utf-8"))
        # check we got 2 elements as expected
        self.assertEqual(len(decoded['result']), 2, "1. Result section of response was expected to have 2 elements but "
                                                    "instead is has {}. Command output object:"
                                                    " {}".format(len(decoded['result']), result))
        # check elements names match expectation
        found_first_backup = False
        for backup_job in decoded['result']:
            if backup_job['name'] == 'first_backup':
                found_first_backup = True
                # check state of backup is "running"
                self.assertEqual(backup_job['state'], 'running', "Backup job does not have state='running'. Command "
                                                                 "output object: {}".format(result))
        self.assertTrue(found_first_backup, "1. 'first_backup' was not found in list of backup jobs. Command output "
                                            "object: {}".format(result))
        # stop running backup
        result = run_shell_cmd(self.cmd + " client backup stop first_backup -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))
        # validate backup has stopped or is stopping
        result = run_shell_cmd(self.cmd + " client backup list -c " + self.client_config_file_path + " --json")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # output is JSON
        decoded = json.loads(result['result'].stdout.decode("utf-8"))
        # check we got 2 elements as expected
        self.assertEqual(len(decoded['result']), 2, "2. Result section of response was expected to have 2 elements but "
                                                    "instead is has {}. Command output object:"
                                                    " {}".format(len(decoded['result']), result))
        # check elements names match expectation
        found_first_backup = False
        first_backup_stopping_or_stopped = False
        for backup_job in decoded['result']:
            if backup_job['name'] == 'first_backup':
                found_first_backup = True
                # check state of backup is "running"
                if backup_job['state'] == 'stopping' or backup_job['state'] == 'stopped':
                    first_backup_stopping_or_stopped = True
        self.assertTrue(found_first_backup, "2. 'first_backup' was not found in list of backup jobs. Command output "
                                            "object: {}".format(result))
        self.assertTrue(first_backup_stopping_or_stopped, "Backup job does not have state='stopping' or state='stopped'"
                                                          ". Command output object: {}".format(result))

    # ./cloudbackup client backup start first_backup -c client_config.yaml -u user_with_read_access -p password
    # returns exit code 1 and expected error msg ; we also get to test command line overrides work
    def test_cmd_client_backup_start2(self):
        result = run_shell_cmd(self.cmd + " client backup start first_backup -c " + self.client_config_file_path +
                               " -u " + self.username2 + " -p " + self.password2)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))
        found_expected_error = False
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            if 'does not have access to'.lower() in line.lower():
                found_expected_error = True
        self.assertTrue(found_expected_error)

    # ./cloudbackup client backup start INVALID_BACKUP_NAME -c client_config.yaml returns exit code 1 and expected
    # error msg
    def test_cmd_client_backup_start3(self):
        result = run_shell_cmd(self.cmd + " client backup start INVALID_BACKUP_NAME -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))
        found_expected_error = False
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            if 'No backup job was found matching name'.lower() in line.lower():
                found_expected_error = True
        self.assertTrue(found_expected_error)

    # ./cloudbackup client backup stop first_backup -c client_config.yaml fails for stopped backups
    def test_cmd_client_backup_stop1(self):
        result = run_shell_cmd(self.cmd + " client backup stop first_backup -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))
        found_expected_error = False
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            if 'is not running'.lower() in line.lower():
                found_expected_error = True
        self.assertTrue(found_expected_error)

    # ./cloudbackup client backup stop first_backup -c client_config.yaml fails for inexisting backup jobs
    def test_cmd_client_backup_stop2(self):
        result = run_shell_cmd(self.cmd + " client backup stop INVALID_BACKUP_NAME -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))
        found_expected_error = False
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            if 'is not running'.lower() in line.lower():
                found_expected_error = True
        self.assertTrue(found_expected_error, "Command output object: {}".format(result))


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestCliAdvanced)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
