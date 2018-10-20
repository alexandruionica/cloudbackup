#!/usr/bin/env python
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


class TestCliBasics(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_cli_basics')
        # client - config file
        tmphandle, self.client_config_file_path = tempfile.mkstemp(suffix='_integration_tests_client_config_file.yaml')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(working_client_config_file_content)
        tmpfile.close()

    def tearDown(self):
        # remove config file and any tmp dirs required by config file statements
        for entry in self.to_delete:
            if os.path.exists(entry):
                if os.path.isdir(entry):
                    shutil.rmtree(entry)
                else:
                    os.remove(entry)
        if os.path.exists(self.client_config_file_path):
            os.remove(self.client_config_file_path)

    # command with no arguments should return exit code 1
    def test_cmd_no_arguments(self):
        result = run_shell_cmd(self.cmd)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup server config dump -c config.yaml shows only obfuscated passwords
    def test_cmd_server_dump_obfuscated_passwords(self):
        result = run_shell_cmd(self.cmd + " server config dump -c " + self.server_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            if 'pass' in line.lower():
                pass_field = line.split(':')[1]
                password = pass_field.split('"')[1]
                re_result = re.match('^(\*+)|()$', password)
                self.assertTrue(re_result, "output from './cloudbackup server config dump -c config.yaml' has on line "
                                           "{} a password which doesn't seem to be "
                                           "obfuscated:\n{}".format(line_num, line))
            line_num += 1

    # ./cloudbackup server config dump -c config.yaml produces at least 89 lines of output
    def test_cmd_server_dump_output_length(self):
        result = run_shell_cmd(self.cmd + " server config dump -c " + self.server_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            line_num += 1
        self.assertGreater(line_num, 88, "Expected output from {} to be at least 88 lines long. Command output object: "
                                         "{}".format(cmd_default, result))

    # ./cloudbackup server config validate -c config.yaml  returns 0 with valid config file
    def test_cmd_validate_server_config1(self):
        result = run_shell_cmd(self.cmd + " server config validate -c " + self.server_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup server config validate -c config.yaml  returns 1 with invalid config file (valid yaml, invalid logic)
    def test_cmd_validate_server_config2(self):
        # load valid yaml data from tmp config, alter it a bit to cause validation to fail and then write it back
        with open(self.server_config_file_path) as fd:
            parsed = yaml.load(fd)
            parsed['backup'][0]['encrypt'] = True
            parsed['backup'][0]['encrypt_pass'] = ''
            parsed['backup'][1]['encrypt'] = True
            parsed['backup'][1]['encrypt_pass'] = ''
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))

        result = run_shell_cmd(self.cmd + " server config validate -c " + self.server_config_file_path)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup server config example produces valid yaml, at least 60 lines long
    def test_cmd_server_example_config1(self):
        result = run_shell_cmd(self.cmd + " server config example")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            line_num += 1
        self.assertGreater(line_num, 60, "Expected output from {} to be at least 60 lines long. Command output object: "
                                         "{}".format(cmd_default, result))
        # if this raises and exception then we got a problem
        yaml.load(result['result'].stdout.decode("utf-8"))

    # ./cloudbackup server config example produces valid config file
    def test_cmd_server_example_config2(self):
        result = run_shell_cmd(self.cmd + " server config example")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # adjust server config file
        loaded_config = yaml.load(result['result'].stdout.decode("utf-8"))
        # we need to adjust data_dir as the path may not exist on the CI/CD systems
        loaded_config['data_dir'] = './tmp/'
        # skip https certificate path validation
        loaded_config['https']['enabled'] = False
        tmpfile = open(self.server_config_file_path, "w")
        tmpfile.write(yaml.dump(loaded_config))
        tmpfile.close()
        # validate config file created from config example
        result = run_shell_cmd(self.cmd + " server config validate -c " + self.server_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup misc hash-password
    def test_cmd_hash_password(self):
        test_password = 'ui7Ahtae\Quai5ia\W;oo"ri'
        proc = run_interactive_shell_cmd(self.cmd + " misc hash-password")
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

    # ./cloudbackup server start -c /path/to/temporary/config.yaml
    def test_cmd_server_start(self):
        base_url = "http://127.0.0.1:8080"
        daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=base_url)
        stopped, _, _ = daemon.stop()
        self.assertTrue(stopped)

    # ./cloudbackup server start -c /path/to/temporary/config.yaml
    def test_cmd_server_start_logging1(self):
        base_url = "http://127.0.0.1:8080"
        daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=base_url)
        output = daemon.get_output(3)
        found_info_messages = False
        # the last element of the below split will be "" ; for for 3 fetched lines we get 4 elements
        for line in output.split('\n'):
            try:
                decoded = json.loads(line)
                if decoded['level'] == 'info':
                    found_info_messages = True
            except json.decoder.JSONDecodeError:
                continue
        stopped, _, _ = daemon.stop()
        self.assertTrue(stopped)
        self.assertTrue(found_info_messages, "Did not manage to find any 'info' log level messages. Output from "
                                             "daemon was: {}".format(output))

    # ./cloudbackup server start -c /path/to/temporary/config.yaml -d
    def test_cmd_server_start_logging2(self):
        # Skip test on windows as it seems some kind of bug is preventing startup when -d is used
        if platform.system() == 'Windows':
            return
        base_url = "http://127.0.0.1:8080"
        daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=base_url, extra_options='-d')
        output = daemon.get_output(3)
        found_info_messages = False
        found_debug_messages = False
        # the last element of the below split will be "" ; for for 3 fetched lines we get 4 elements
        for line in output.split('\n'):
            try:
                decoded = json.loads(line)
                if decoded['level'] == 'info':
                    found_info_messages = True
                elif decoded['level'] == 'debug':
                    found_debug_messages = True
            except json.decoder.JSONDecodeError:
                continue
        stopped, _, _ = daemon.stop()
        self.assertTrue(stopped)
        self.assertTrue(found_info_messages, "Did not manage to find any 'info' log level messages. Output from "
                                             "daemon was: {}".format(output))
        self.assertTrue(found_debug_messages, "Did not manage to find any 'debug' log level messages. Output from "
                                              "daemon was: {}".format(output))

    # ./cloudbackup client config validate -c config.yaml  returns 0 with valid config file
    def test_cmd_validate_client_config1(self):
        result = run_shell_cmd(self.cmd + " client config validate -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup client config validate -c config.yaml  returns 1 with invalid config file (valid yaml,
    # invalid logic)
    def test_cmd_validate_client_config2(self):
        # load valid yaml data from tmp config, alter it a bit to cause validation to fail and then write it back
        with open(self.client_config_file_path) as fd:
            parsed = yaml.load(fd)
            parsed['address'] = 'ftp://google.com:21'
        with open(self.client_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))

        result = run_shell_cmd(self.cmd + " client config validate -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 1, "Exit code from {} is not 1. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup client config validate -u testuser -p apassword -a https://127.0.0.5:5050 returns 0 with valid
    # command line config opts
    def test_cmd_validate_client_config3(self):
        result = run_shell_cmd(self.cmd + " client config validate -u testuser -p apassword -a https://127.0.0.5:5050")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup client config validate --username testuser --password apassword --address https://127.0.0.5:5050
    # returns 0 with valid command line config opts (long version)
    def test_cmd_validate_client_config4(self):
        result = run_shell_cmd(self.cmd + " client config validate --username testuser --password apassword "
                                          "--address https://127.0.0.5:5050")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup client config validate -c config.yaml  returns 0 with invalid config file (valid yaml,
    # invalid logic) as long as the invalid elements are overriden on the command line - basically test we can
    # override stuff
    def test_cmd_validate_client_config5(self):
        # load valid yaml data from tmp config, alter it a bit to cause validation to fail and then write it back
        with open(self.client_config_file_path) as fd:
            parsed = yaml.load(fd)
            parsed['address'] = 'ftp://google.com:21'
        with open(self.client_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))

        result = run_shell_cmd(self.cmd + " client config validate -c " + self.client_config_file_path +
                               " --address https://127.0.0.5:5050")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))

    # ./cloudbackup client config dump -c config.yaml shows only obfuscated passwords
    def test_cmd_client_dump_obfuscated_passwords(self):
        result = run_shell_cmd(self.cmd + " client config dump -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            if 'pass' in line.lower():
                pass_field = line.split(':')[1]
                password = pass_field.split('"')[1]
                re_result = re.match('^(\*+)|()$', password)
                self.assertTrue(re_result, "output from './cloudbackup client config dump -c config.yaml' has on line "
                                           "{} a password which doesn't seem to be "
                                           "obfuscated:\n{}".format(line_num, line))
            line_num += 1

    # ./cloudbackup client config dump -c config.yaml produces at least 5 lines of output
    def test_cmd_client_dump_output_length(self):
        result = run_shell_cmd(self.cmd + " client config dump -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            line_num += 1
        self.assertGreater(line_num, 5, "Expected output from {} to be at least 5 lines long. Command output object: "
                                        "{}".format(cmd_default, result))

    # ./cloudbackup client config dump -c config.yaml --address https://127.7.8.5:4050 ends up with --address taking
    # priority over config file value
    def test_cmd_client_dump_output_cli_override1(self):
        result = run_shell_cmd(self.cmd + " client config dump -c " + self.client_config_file_path +
                               " --address https://127.7.8.5:4050")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        decoded = json.loads(result['result'].stdout.decode("utf-8"))
        self.assertEqual(decoded['address'], 'https://127.7.8.5:4050', 'Command line was supposed to override config '
                                                                       'file but config dump shows otherwise:'
                                                                       ' {}'.format(decoded))

    # ./cloudbackup client config dump -c config.yaml --address https://127.7.8.5:4050 ends up with --address taking
    # priority over config file value and also over environment variable value
    def test_cmd_client_dump_output_cli_override2(self):
        os.environ['CLOUDBACKUP_CLIENT_ADDRESS'] = 'https://127.3.3.3:3070'
        result = run_shell_cmd(self.cmd + " client config dump -c " + self.client_config_file_path +
                               " --address https://127.7.8.5:4050")
        os.environ.pop('CLOUDBACKUP_CLIENT_ADDRESS')
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        decoded = json.loads(result['result'].stdout.decode("utf-8"))
        self.assertEqual(decoded['address'], 'https://127.7.8.5:4050', 'Command line was supposed to override config '
                                                                       'file and environment variable but config dump'
                                                                       ' shows otherwise: {}'.format(decoded))

    # ./cloudbackup client config dump -c config.yaml with environment variable CLOUDBACKUP_CLIENT_ADDRESS overrideing
    # config file option
    def test_cmd_client_dump_output_env_override(self):
        os.environ['CLOUDBACKUP_CLIENT_ADDRESS'] = 'https://127.7.8.5:4050'
        result = run_shell_cmd(self.cmd + " client config dump -c " + self.client_config_file_path)
        os.environ.pop('CLOUDBACKUP_CLIENT_ADDRESS')
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        decoded = json.loads(result['result'].stdout.decode("utf-8"))
        self.assertEqual(decoded['address'], 'https://127.7.8.5:4050', 'Command line was supposed to override config '
                                                                       'file but config dump shows otherwise:'
                                                                       ' {}'.format(decoded))

    # ./cloudbackup client config example produces valid yaml, at least 4 lines long
    def test_cmd_client_example_config1(self):
        result = run_shell_cmd(self.cmd + " client config example")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        line_num = 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            line_num += 1
        self.assertGreater(line_num, 3, "Expected output from {} to be at least 3 lines long. Command output object: "
                                        "{}".format(cmd_default, result))
        # if this raises and exception then we got a problem
        yaml.load(result['result'].stdout.decode("utf-8"))

    # ./cloudbackup client config example produces valid config example,
    def test_cmd_client_example_config2(self):
        result = run_shell_cmd(self.cmd + " client config example")
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # adjust client config file
        tmpfile = open(self.client_config_file_path, "w")
        tmpfile.write(result['result'].stdout.decode("utf-8"))
        tmpfile.close()
        # validate config file created from config example
        result = run_shell_cmd(self.cmd + " client config validate -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))


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
