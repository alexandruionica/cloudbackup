#!/usr/bin/env python
import argparse
import logging
import os
import shutil
import subprocess
import sys
import unittest
from common import *


class TestRestAPISwagger(unittest.TestCase):
    def setUp(self):
        self.cmd = cmd_default
        self.config_file_path, self.to_delete = setup_tmp_config_file_and_tmp_dirs(
            suffix='_integration_tests_rest_api_swagger')
        self.base_url = "http://127.0.0.1:8080"
        # the daemon serves /swagger.json as a redirect to the static spec, so we
        # point schemathesis straight at the canonical location
        self.schema_url = self.base_url + "/docs_api/swagger.json"
        # schemathesis fires well over a thousand requests; run the daemon in
        # --quiet mode so its per-request logging does not fill the captured
        # stdout/stderr pipe and stall the server mid-run
        self.daemon = BackupDaemon(config_path=self.config_file_path, base_url=self.base_url,
                                   extra_options="--quiet")

    def tearDown(self):
        self.daemon.kill()
        # remove config file and any tmp dirs required by config file statements
        for entry in self.to_delete:
            if os.path.exists(entry):
                if os.path.isdir(entry):
                    shutil.rmtree(entry)
                else:
                    os.remove(entry)

    def test_swagger_conformance_unauthenticated(self):
        """
        Validate that the live API conforms to its published Swagger 2.0 spec.

        We deliberately run schemathesis WITHOUT credentials: every authenticated
        operation must reject the request with the documented 401 before doing any
        work, so all 20 operations get exercised while the stateful backup daemon
        stays untouched (no random fuzzing reaches backup/restore/config writes).
        Schemathesis still validates that the served spec is well formed and that
        the returned status codes, content types and response bodies match the
        spec, which is a strict superset of the old unauthorized-only check.
        """
        # schemathesis is installed as a console script in the same virtualenv as
        # the interpreter running this test
        schemathesis_bin = os.path.join(os.path.dirname(sys.executable), "schemathesis")
        cmd = [
            schemathesis_bin, "run", self.schema_url,
            "--checks", "all",
            "--max-examples", "5",
            "--request-timeout", "5",
        ]
        logging.info("Running swagger conformance check: {}".format(" ".join(cmd)))
        # schemathesis prints Unicode glyphs (box drawing, status marks). Force
        # UTF-8 for the child so it does not crash writing to a non-UTF-8 console
        # encoding (cp1252 on Windows), and decode the captured output as UTF-8.
        env = os.environ.copy()
        env["PYTHONIOENCODING"] = "utf-8"
        result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                encoding="utf-8", errors="replace", env=env)
        stopped, _, _ = self.daemon.stop()
        self.assertTrue(stopped, "Backup daemon already stopped. Something must have gone wrong")
        self.assertEqual(
            result.returncode, 0,
            "schemathesis reported API/spec conformance failures:\n{}".format(result.stdout))


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
