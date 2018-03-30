#!/usr/bin/env python
import argparse
import bcrypt
import logging
import os
import re
import sys
import tempfile
import unittest
import yaml
from common import *
from pprint import pprint


class TestCliBasics(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        tmphandle, self.config_file_path = tempfile.mkstemp(suffix='_integration_tests_config_file.yaml')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(working_config_file_content)
        tmpfile.close()

    def tearDown(self):
        if os.path.exists(self.config_file_path):
            os.remove(self.config_file_path)

    # command with no arguments should return exit code 1
    def test_cmd_no_arguments(self):
        result = run_shell_cmd(self.cmd)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup config dump -c config.yaml shows only obfuscated passwords
    def test_cmd_dump_obfuscated_passwords(self):
        result = run_shell_cmd(self.cmd + " config dump -c " + self.config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            if 'pass' in line.lower():
                pass_field = line.split(':')[1]
                password = pass_field.split('"')[1]
                re_result = re.match('^(\*+)|()$', password)
                self.assertTrue(re_result, "output from './cloudbackup config dump -c config.yaml' has on line {} a"
                                           " password which doesn't seem to be obfuscated:\n{}".format(line_num, line))
            line_num += 1

    # ./cloudbackup config dump -c config.yaml produces at least 89 lines of output
    def test_cmd_dump_output_length(self):
        result = run_shell_cmd(self.cmd + " config dump -c " + self.config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            line_num += 1
        self.assertGreater(line_num, 88, "Expected output from {} to be at least 88 lines long. Command output object: "
                                         "{}".format(cmd_default, result))

    # ./cloudbackup config validate -c config.yaml  returns 0 with valid config file
    def test_cmd_validate_config1(self):
        result = run_shell_cmd(self.cmd + " config validate -c " + self.config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup config validate -c config.yaml  returns 1 with invalid config file (valid yaml, invalid logic)
    def test_cmd_validate_config2(self):
        # load valid yaml data from tmp config, alter it a bit to cause validation to fail and then write it back
        with open(self.config_file_path) as fd:
            parsed = yaml.load(fd)
            parsed['backup'][0]['encrypt'] = True
            parsed['backup'][0]['encrypt_pass'] = ''
            parsed['backup'][1]['encrypt'] = True
            parsed['backup'][1]['encrypt_pass'] = ''
        with open(self.config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))

        result = run_shell_cmd(self.cmd + " config validate -c " + self.config_file_path)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup config example produces valid yaml, at least 60 lines long
    def test_cmd_example_config1(self):
        result = run_shell_cmd(self.cmd + " config example")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            line_num += 1
        self.assertGreater(line_num, 60, "Expected output from {} to be at least 60 lines long. Command output object: "
                                         "{}".format(cmd_default, result))
        # if this raises and exception then we got a problem
        yaml.load(result['result'].stdout.decode("utf-8"))

    # ./cloudbackup hash-password
    def test_cmd_hash_password(self):
        test_password = 'ui7Ahtae\Quai5ia\W;oo"ri'
        proc = run_interactive_shell_cmd(self.cmd + " hash-password")
        stdout_data, stderr_data = proc.communicate(str.encode(test_password + '\n'))
        if proc.poll() is None:
            proc.kill()
        self.assertEqual(proc.returncode, 0, "Return code from {} is not 0 but {}"
                         .format(self.cmd + " hash-password", proc.returncode))
        for line in stdout_data.decode("utf-8").split('\n'):
            if 'The hashed password is:' in line:
                re_result = re.search('\$2.*', line)
                self.assertTrue(re_result)
                bcrypthash = re_result.group(0).strip()
                # check that generated hash matches initial password
                self.assertTrue(bcrypt.checkpw(str.encode(test_password), str.encode(bcrypthash)))

    # ./cloudbackup start -c /path/to/temporary/config.yaml
    def test_cmd_start(self):
        base_url = "http://127.0.0.1:8080"
        daemon = BackupDaemon(config_path=self.config_file_path, base_url=base_url)
        self.assertTrue(daemon.stop())


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestCliBasics)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
