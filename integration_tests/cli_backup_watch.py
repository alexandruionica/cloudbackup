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
import shutil
import sys
import tempfile
import unittest
import yaml
from common import *
from pprint import pprint


class TestCliBackupWatch(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        self.server_config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_cli_backup_watch_')
        self.data_dir = self.to_delete[1]
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
        if os.path.exists(self.tmpdir):
            shutil.rmtree(self.tmpdir)

    # ./cloudbackup client backup start first_backup --watch -c client_config.yaml     works
    def test_cmd_client_backup_start_and_watch1(self):
        result = run_shell_cmd(self.cmd + " client backup start first_backup --watch -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        last_line = result['result'].stdout.decode("utf-8").split('\n')[-2]
        self.assertEqual(last_line, "Backup job has finished")

    # ./cloudbackup client backup start first_backup --watch --json -c client_config.yaml     works
    def test_cmd_client_backup_start_and_watch2(self):
        result = run_shell_cmd(self.cmd + " client backup start first_backup --watch --json -c " +
                               self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        num_files, num_dirs = 0, 0
        for fname in self.filelist:
            if self.filelist[fname] == "dir":
                num_dirs += 1
            elif self.filelist[fname] == "file":
                num_files += 1
        last_line = result['result'].stdout.decode("utf-8").split('\n')[-2]
        self.assertEqual(last_line, "Backup job has finished")

        # decode output except last line
        dryrun_examined = {}
        not_decoded = 0
        line_count = 0
        line_buff = ""
        examined_files, excluded_files_or_dirs, examined_directories, errors_encountered = 0, 0, 0, 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            line_count += 1
            # first 8 lines of output are a pretty formatted json (multi line) while the following are single line
            if line_count < 9:
                line_buff += line
                if line_count == 8:
                    line = line_buff
                else:
                    continue
            try:
                decoded = json.loads(line)
            except json.decoder.JSONDecodeError:
                not_decoded += 1
                continue
            if "code" in decoded:
                # this is the first, multi line json, so we'll ignore it
                continue
            if decoded["error"] != "":
                errors_encountered += 1
                continue
            if decoded["operation_type"] == "excluded":
                excluded_files_or_dirs += 1
                continue
            if decoded['type'] == "directory":
                dryrun_examined[decoded["name"]] = 'dir'
                examined_directories += 1
            else:
                if decoded["type"] == "file":
                    examined_files += 1
                dryrun_examined[decoded["name"]] = decoded['type']
        self.assertEqual(2, not_decoded,
                         "More than two lines in the json output could not be decoded. It is expected"
                         " that 1 line starting with 'Completed run:' and the last (empty) line can't"
                         " be json decoded")
        # add to the list of generated files also the top level dir. This because the dryrun will include it
        self.filelist[self.tmpdir] = 'dir'
        # add to the list of generated files also the compressed copy of the database as this is also uploaded
        self.filelist[self.data_dir + os.sep + "first_backup.sqlite.gz"] = "file"
        # in case the dicts don't match, show the full diff
        self.maxDiff = None
        # add the tmp copy of the config file to the dict too:
        for k in dryrun_examined.keys():
            if "cloudbackup_configuration_file_copy" in k:
                self.filelist[k] = dryrun_examined[k]
        self.assertDictEqual(self.filelist, dryrun_examined)

        # add +1 due to also having the DB copy mandatory included and +1 as the copy of the config file gets uploaded
        self.assertEqual(num_files + 1 + 1, examined_files)
        # top level dir counts too so we increment with 1 the initial list of directories
        self.assertEqual(num_dirs + 1, examined_directories)
        self.assertEqual(0, excluded_files_or_dirs)
        self.assertEqual(0, errors_encountered)

    # ./cloudbackup client backup watch first_backup -c client_config.yaml     works
    def test_cmd_client_backup_watch1(self):
        # first start the backup job so we can then attach and watch
        result = run_shell_cmd(self.cmd + " client backup start first_backup -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # attach to running backup and watch
        result = run_shell_cmd(self.cmd + " client backup watch first_backup -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        num_files, num_dirs = 0, 0
        for fname in self.filelist:
            if self.filelist[fname] == "dir":
                num_dirs += 1
            elif self.filelist[fname] == "file":
                num_files += 1
        last_line = result['result'].stdout.decode("utf-8").split('\n')[-2]
        self.assertEqual(last_line, "Backup job has finished")

    # ./cloudbackup client backup watch first_backup --json -c client_config.yaml     works
    def test_cmd_client_backup_watch2(self):
        # first start the backup job so we can then attach and watch
        result = run_shell_cmd(self.cmd + " client backup start first_backup -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        # attach to running backup and watch
        result = run_shell_cmd(self.cmd + " client backup watch first_backup --json -c " + self.client_config_file_path)
        self.assertEqual(result['result'].returncode, 0, "Exit code from {} is not 0. Command output object: "
                                                         "{}".format(cmd_default, result))
        num_files, num_dirs = 0, 0
        for fname in self.filelist:
            if self.filelist[fname] == "dir":
                num_dirs += 1
            elif self.filelist[fname] == "file":
                num_files += 1
        last_line = result['result'].stdout.decode("utf-8").split('\n')[-2]
        self.assertEqual(last_line, "Backup job has finished")

        # decode output except last line
        dryrun_examined = {}
        not_decoded = 0
        examined_files, excluded_files_or_dirs, examined_directories, errors_encountered = 0, 0, 0, 0
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            try:
                decoded = json.loads(line)
            except json.decoder.JSONDecodeError:
                not_decoded += 1
                continue
            if "code" in decoded:
                # this is the first, multi line json, so we'll ignore it
                continue
            if decoded["error"] != "":
                errors_encountered += 1
                continue
            if decoded["operation_type"] == "excluded":
                excluded_files_or_dirs += 1
                continue
            if decoded['type'] == "directory":
                dryrun_examined[decoded["name"]] = 'dir'
                examined_directories += 1
            else:
                if decoded["type"] == "file":
                    examined_files += 1
                dryrun_examined[decoded["name"]] = decoded['type']
        self.assertEqual(2, not_decoded,
                         "More than two lines in the json output could not be decoded. It is expected"
                         " that 1 line starting with 'Completed run:' and the last (empty) line can't"
                         " be json decoded")
        # add to the list of generated files also the top level dir. This because the dryrun will include it
        self.filelist[self.tmpdir] = 'dir'
        # add to the list of generated files also the compressed copy of the database as this is also uploaded
        self.filelist[self.data_dir + os.sep + "first_backup.sqlite.gz"] = "file"
        # in case the dicts don't match, show the full diff
        self.maxDiff = None
        # add the tmp copy of the config file to the dict too:
        for k in dryrun_examined.keys():
            if "cloudbackup_configuration_file_copy" in k:
                self.filelist[k] = dryrun_examined[k]
        self.assertDictEqual(self.filelist, dryrun_examined)

        # add +1 due to also having the DB copy mandatory included and +1 as the copy of the config file gets uploaded
        self.assertEqual(num_files + 1 + 1, examined_files)
        # top level dir counts too so we increment with 1 the initial list of directories
        self.assertEqual(num_dirs + 1, examined_directories)
        self.assertEqual(0, excluded_files_or_dirs)
        self.assertEqual(0, errors_encountered)


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestCliBackupWatch)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
