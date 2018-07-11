#!/usr/bin/env python
#
# CLI Tests which require a server to be running as a prerequisite of the test
#
#
import argparse
import bcrypt
import copy
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


class TestCliBackupValidate(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.username = 'testuser1'
        self.password = 'HV}H/y?<9$]Z5N4N'
        self.username2 = 'testuser2'
        self.password2 = 'Oonaawai8Eep]eethe8eefa$'
        # server - config file
        tmphandle, self.server_config_file_path = tempfile.mkstemp(suffix='_integration_tests_server_config_file.yaml')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(working_server_config_file_content)
        tmpfile.close()
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
        # make 1 folder unreadable - os.chmod doesn't work on Windows
        if platform.system() != 'Windows':
            os.chmod(self.tmpdir + os.sep + "dir1" + os.sep + "dir5", 0o0000)
        with open(self.server_config_file_path, "w") as fd:
            fd.write(yaml.dump(parsed))
        # start server
        self.base_url = "http://127.0.0.1:8080"
        self.daemon = BackupDaemon(config_path=self.server_config_file_path, base_url=self.base_url)

    def tearDown(self):
        if os.path.exists(self.server_config_file_path):
            os.remove(self.server_config_file_path)
        if os.path.exists(self.client_config_file_path):
            os.remove(self.client_config_file_path)
        self.daemon.kill()
        # make 1 dir from unreadable back to readable - os.chmod doesn't work on Windows
        if platform.system() != 'Windows':
            os.chmod(self.tmpdir + os.sep + "dir1" + os.sep + "dir5", 0o0755)
        if os.path.exists(self.tmpdir):
            shutil.rmtree(self.tmpdir)

    # ./cloudbackup client backup dryrun first_backup --json -c client_config.yaml     works
    def test_cmd_client_backup_dryrun(self):
        # don't run on MS Windows this test
        if platform.system() == 'Windows':
            return
        result = run_shell_cmd(self.cmd + " client backup dryrun first_backup --json -c " +
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
        re_result = re.search('^Completed run: ([0-9]+) examined files, ([0-9]+) examined directories, ([0-9]+) '
                              'excluded files or directories, ([0-9]+) errors encountered', last_line)
        # check regex worked
        self.assertTrue(re_result)
        examined_files = int(re_result.group(1))
        examined_directories = int(re_result.group(2))
        excluded_files_or_dirs = int(re_result.group(3))
        errors_encountered = int(re_result.group(4))
        # we made a folder containing 2 files unreadable so those 2 need to be discounted for
        self.assertEqual(num_files - 2, examined_files)
        # top level dir counts too so we increment with 1 the initial list of directories
        self.assertEqual(num_dirs + 1, examined_directories)
        self.assertEqual(0, excluded_files_or_dirs)
        self.assertEqual(1, errors_encountered)
        # decode output except last line
        dryrun_examined = {}
        not_decoded = 0
        errored_elements = []
        for line in result['result'].stdout.decode("utf-8").split('\n'):
            try:
                decoded = json.loads(line)
            except json.decoder.JSONDecodeError:
                not_decoded += 1
                continue
            if decoded['excluded']:
                continue
            if decoded['error'] != "":
                errored_elements.append(decoded['name'])
                continue
            if decoded['type'] == "directory":
                dryrun_examined[decoded["name"]] = 'dir'
            else:
                dryrun_examined[decoded["name"]] = decoded['type']
        self.assertEqual(2, not_decoded, "More than two lines in the json output could not be decoded. It is expected"
                                         " that 1 line starting with 'Completed run:' and the last (empty) line can't"
                                         " be json decoded")
        # add to the list of generated files also the top level dir. This because the dryrun will include it
        self.filelist[self.tmpdir] = 'dir'
        # in case the dicts don't match, show the full diff
        self.maxDiff = None
        # remove errored out items from the initial dir we use for comparison
        filelist_copy = copy.copy(self.filelist)
        for element in errored_elements:
            for item in self.filelist:
                if element == item or item.startswith(element + os.sep):
                    filelist_copy.pop(item)
                    continue
        self.assertDictEqual(filelist_copy, dryrun_examined)


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

    suite = unittest.TestLoader().loadTestsFromTestCase(TestCliBackupValidate)
    result = unittest.TextTestRunner(verbosity=verbosity, failfast=False).run(suite,)
    if result.failures:
        sys.exit(1)


if __name__ == '__main__':
    main()
