#!/usr/bin/env python
import asyncio
import asyncore
import hashlib
import json
import logging
import platform
import os
import re
import requests
import socket
import shlex
import shutil
import subprocess
import threading
import time
import tempfile
import quopri
import unittest
import yaml
from aiosmtpd.controller import Controller


logging.basicConfig(format='%(asctime)-15s %(levelname)s: %(message)s', level=logging.INFO)

if platform.system().lower() == 'windows':
    cmd_default = r".\cloudbackup.exe"
else:
    cmd_default = "./cloudbackup"
working_server_config_file_content = '''# global settings affect all backups and can't be specified per backup with different values
# section specific settings are repetitive and can't be overridden by globals
# clarity and safety are paramount to the design so repeating a particular key - value over and over is acceptable
#
#
data_dir: ./tmp/
#defaults to webstatic/ relative to where the cloudbackup binary is located
html_dir: webstatic
user:
  - name: testuser1
    # bcrypt hash of password  "HV}H/y?<9$]Z5N4N" - use ./cloudbackup misc hash-password to hash passwords
    pass: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
    # can be either 'read' or 'write' . 'write' basically gives access to all the API while 'read' only to read-only
    #  operations so for example it excludes things starting/stopping backups or adjusting the configuration
    access: write
  - name: testuser2
    # bcrypt hash of password  "Oonaawai8Eep]eethe8eefa$"
    pass: $2a$05$Pgdwe14mHjOQ33C5LahmmugCY85Yfqlkj2rGvbDMGCDXKKwmhbwVC
    access: read
# host and port for the HTTP server; if HTTPS server is enabled then http server is automatically disabled.
# By default HTTP server is enabled and HTTPS is disabled
#http:
#  bind_address: "127.0.0.1:8080"
#https:
#  enabled: true
#  bind_address: "127.0.0.1:8443"
#  ssl_cert_path: /etc/ssl/cert.crt
#  ssl_key_path: /etc/ssl/cert.key
backup:
  - name: first_backup
    paths:
      - /something
      - /var/lib
    exclusions:
      - /something/else
      - /var/lib/mysql
    target:
      - name: aws_1
        type: test_null
        bucket: 'myawesome-backup'
        prefix: 'backup/backups-for-server-51'
        parameters:
          - name: AWS_ACCESS_KEY_ID
            value: AKIAIOSFODNN7EXAMPLE
          - name: AWS_SECRET_ACCESS_KEY
            value: wJalrXUtnFEMI/K7MDENG/bPxRfiCEXAMPLEKEY
          - name: storage_class
            value: STANDARD
    schedule:
      - '05 01 * * *'
  - name: second_backup
    paths:
      - /var/log
      - /var/www/html/data/
    # do not follow symbolic links (defaults to true)
    dereference: false
    # use the file's checksum in order to establish if a backup is needed (defaults to false)
    checksum: true
    target:
      - name: aws_2
        type: test_null
        bucket: 'some-stuff-goes-here'
        prefix: 'backup/backups-for-server-51'
        parameters:
          - name: storage_class
            value: STANDARD
      - name: google_1
        type: gcp_storage
        bucket: 'my-google-bucket'
        prefix: 'backup/backups-for-server-51'
    encrypt: true
    encrypt_pass: '044ewfsoi423092l;dfksdl;fksl;dfks;ld0492'
    schedule:
      - '00 08 01 * *'
      - '00 08 06 * *'
    # defaults to 0 which means unlimited number of versions
    versions_max_num: 10
    # defaults to 0 which means unlimited age
    versions_max_age: 6w
'''

working_client_config_file_content = '''---
username: testuser1
password: 'HV}H/y?<9$]Z5N4N'
address: http://127.0.0.1:8080
'''


def client_config_content(base_url):
    """Client config YAML for testuser1 pointing at the daemon reachable at $base_url."""
    return working_client_config_file_content.replace("address: http://127.0.0.1:8080", "address: " + base_url, 1)


def write_client_config(base_url, suffix='_integration_tests_client_config_file.yaml'):
    """Write a client config for $base_url to a temp file and return its path (caller deletes it)."""
    tmphandle, path = tempfile.mkstemp(suffix=suffix)
    with os.fdopen(tmphandle, "w") as fd:
        fd.write(client_config_content(base_url))
    return path


def find_free_port(ipaddr='127.0.0.1'):
    """Ask the OS for a currently free TCP port on $ipaddr. The caller binds it shortly after."""
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.bind((ipaddr, 0))
        return s.getsockname()[1]
    finally:
        s.close()


def free_base_url():
    """
    Base URL (http://127.0.0.1:<port>) on a port that is free right now. BackupDaemon() rewrites the server
    config's http.bind_address to match, so tests no longer share one hardcoded port: a stale daemon or a
    developer's own server on 8080 cannot break the run, and independent test processes can run side by side.
    """
    return "http://127.0.0.1:{}".format(find_free_port())


def requires_env(*names):
    """
    Class or method decorator: skip the test(s) unless every environment variable in $names is set and
    non-empty. Used by the cloud tier so a machine without credentials reports skips, not failures.
    """
    missing = [n for n in names if not os.environ.get(n)]
    return unittest.skipUnless(not missing, "missing environment variable(s): {}".format(", ".join(missing)))


def decode_json_stream(text):
    """
    Decode a stream of concatenated JSON documents, each possibly pretty-printed across several lines and
    possibly interleaved with plain-text lines (for example the trailing 'Backup job has finished').
    :return: (list of decoded documents in order, list of plain-text lines that were not JSON)
    """
    decoder = json.JSONDecoder()
    docs, plain = [], []
    pos, n = 0, len(text)
    while pos < n:
        while pos < n and text[pos] in ' \t\r\n':
            pos += 1
        if pos >= n:
            break
        try:
            obj, end = decoder.raw_decode(text, pos)
        except json.JSONDecodeError:
            nl = text.find('\n', pos)
            if nl == -1:
                nl = n
            plain.append(text[pos:nl])
            pos = nl + 1
            continue
        docs.append(obj)
        pos = end
    return docs, plain


verbose = False


def run_shell_cmd(cmd):
    """
    Simple wrapper to run shell command
    :param cmd: command to run
    :return: { 'result': None/subprocess.CompletedProcess,
               'exception: None/exception ..}
    """
    logging.info('Running shell command: {}'.format(cmd))
    try:
        return {'result': subprocess.run(cmd, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE),
                'exception': None,
                }
    except subprocess.CalledProcessError as e:
        logging.exception(e.output)
        return {'result': None,
                'exception': e,
                }


def run_interactive_shell_cmd(cmd):
    """
    Wrapper to start a shell command which then keeps running
    :param cmd: command to run
    :return: { 'result': None/subprocess.CompletedProcess,
               'exception: None/exception ..}
    """
    logging.info('Running interactive shell command: {}'.format(cmd))
    return subprocess.Popen(cmd, shell=True, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)


def set_http_bind_address(config_path, base_url):
    """
    Point the server config at $config_path to listen on the host:port of $base_url. An https:// URL
    enables the HTTPS listener on that address (cert/key paths must already be in the config, and the
    daemon then disables plain HTTP); an http:// URL sets http.bind_address.
    """
    host_port = base_url.split('//', 1)[1].rstrip('/')
    with open(config_path) as fd:
        parsed = yaml.load(fd, Loader=yaml.SafeLoader)
    if base_url.startswith('https://'):
        https = parsed.get('https') or {}
        https['enabled'] = True
        https['bind_address'] = host_port
        parsed['https'] = https
    else:
        http = parsed.get('http') or {}
        http['bind_address'] = host_port
        parsed['http'] = http
    with open(config_path, "w") as fd:
        fd.write(yaml.dump(parsed))


class BackupDaemon(object):
    """
    Start cloudbackup daemon
    """
    def __init__(self, config_path, base_url=None, cmd=cmd_default, extra_options="", tls_ca=None):
        """
        start backup daemon
        Wrapper to start a shell command which then keeps running
        :param cmd: command to run
        :param base_url: where the API server will be reachable, for example "http://127.0.0.1:8080". The server
            config at $config_path is rewritten so its http.bind_address matches. When None, a free port is picked
            (see free_base_url()). The effective value is available afterwards as self.base_url.
        :param extra_options: extra options to pass to the backup server(Daemon)
        :param tls_ca: for an https:// base_url, path of the CA / self-signed certificate to trust when probing
        """
        if base_url is None:
            base_url = free_base_url()
        self.base_url = base_url
        self.config_path = config_path
        self.tls_ca = tls_ca
        set_http_bind_address(config_path, base_url)
        # check ip:port is available
        wait_for_socket(base_url)

        if platform.system().lower() == 'windows':
            # Windows needs absolute paths because we're not running the command in a shell
            full_cmd = os.path.abspath(cmd) + ' server start -c {} '\
                .format(os.path.abspath(config_path)) + extra_options
            cmd_with_args = full_cmd
        else:
            full_cmd = cmd + ' server start -c {} '.format(config_path) + extra_options
            cmd_with_args = shlex.split(full_cmd)
        logging.info('Running the backup daemon using: {}'.format(full_cmd))
        self.proc = subprocess.Popen(cmd_with_args, shell=False, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.PIPE, universal_newlines=True, bufsize=1)
        # there is a slight delay between daemon start and http becoming available so we need to ensure it is
        #   available before tests are attempted
        if not check_api_server_ready(base_url, verify=tls_ca if tls_ca else True):
            _, stderr, stdout = self.stop(get_output=True)
            logging.error("Could not connect to API server after starting the daemon. Daemon's stdout was: {} "
                          "\n and stderr was: {}".format(stderr, stdout))
            raise requests.exceptions.ConnectionError(
                "Could not connect to CloudBackup API server at {}".format(base_url))

    def kill(self, max_count=20, sleep_time=0.1):
        """
        kill daemon
        :return: True on success, False if process already exited
        """
        if self.proc.poll() is None:
            self.proc.kill()
            counter = 0
            while counter < max_count:
                if self.proc.poll() is None:
                    time.sleep(sleep_time)
                    counter += 1
                    continue
                else:
                    counter = 0
                    break

            if counter == max_count:
                raise Exception(
                    "Attempt to kill CloudBackup process did not succeed. Checked process status {} times, at {} "
                    "seconds interval".format(counter, sleep_time))
            # close file descriptors for stdin/stdout/stderr
            self.proc.stderr.close()
            self.proc.stdout.close()
            self.proc.stdin.close()
            return True
        else:
            return False

    def stop(self, max_count=20, sleep_time=0.1, get_output=False):
        """
        stop daemon using terminate()
        :return: tuple with (True on success / False if process already exited, stderr, stdout)
                Stderr / stdout will be replaced with empty strings if get_output == False
        """
        stderr = ""
        stdout = ""
        if self.proc.poll() is None:
            self.proc.terminate()
            counter = 0
            while counter < max_count:
                if self.proc.poll() is None:
                    time.sleep(sleep_time)
                    counter += 1
                    continue
                else:
                    counter = 0
                    break

            if counter == max_count:
                raise Exception(
                    "Attempt to stop(terminate not kill) CloudBackup process did not succeed. Checked process status"
                    " {} times, at {} seconds interval".format(counter, sleep_time))
            if get_output:
                stdout, stderr = self.proc.communicate()
            # close file descriptors for stdin/stdout/stderr
            self.proc.stderr.close()
            self.proc.stdout.close()
            self.proc.stdin.close()
            return True, stderr, stdout
        else:
            if get_output:
                stdout, stderr = self.proc.communicate()
            return False, stderr, stdout

    def is_running(self):
        """
        check if daemon still running
        :return: True if running, False if exited
        """
        if self.proc.poll() is None:
            return True
        else:
            return False

    def get_output(self, num_lines=1):
        """
        get output from process
        :param num_lines: how many lines of output to read. If more lines are requested then available the this will
        block until lines are produced by the process
        :return: string holding output
        """
        read_lines = 0
        total_output = None
        while read_lines < num_lines:
            read_lines += 1
            output = self.proc.stdout.readline()
            if output:
                if total_output:
                    total_output = total_output + output
                else:
                    total_output = output
            if self.proc.poll() is not None:
                break
        return total_output


def wait_for_socket(base_url, max_count=600, sleep_seconds=0.1):
    """
    Attempt $max_count times, with $sleep_seconds seconds sleep to bind on the listening IP:port. This is to give
    time for whatever keeps the port open to close it before we attempt to run the test
    :return:
    """
    ipaddr = base_url.split(':')[1].strip('/')
    port = int(base_url.split(':')[2])
    counter = 0
    while counter < max_count:
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        # Use SO_REUSEADDR to allow immediate restart
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            s.bind((ipaddr, port))
        except OSError:
            s.close()
            time.sleep(sleep_seconds)
            counter += 1
            continue
        else:
            s.close()
            counter = 0
            break
    if counter == max_count:
        raise OSError(
            "Something else is already bound to {}:{} . Attempted unsuccessfully to bind {} "
            "times for a total of {} seconds wait".format(ipaddr, port, counter, counter * sleep_seconds))


def check_api_server_ready(url, max_count=20, sleep_seconds=0.1, verify=True):
    """
    Attempt $max_count times, with $sleep_seconds seconds sleep to get / from the http server. This is to give time
       to start up
    :param verify: passed to requests (True, or the path of a certificate to trust for https URLs)
    :return: False if it did not start up during wait time, True if succeeded
    """
    counter = 0
    while counter < max_count:
        try:
            requests.get(url, verify=verify)
        except requests.exceptions.ConnectionError:
            time.sleep(sleep_seconds)
            counter += 1
            continue
        else:
            counter = 0
            break
    if counter == max_count:
        logging.error(
            "Could not connect to CloudBackup API server at {} after {} attempts for a total of {} "
            "seconds".format(url, counter, counter * sleep_seconds))
        return False
    else:
        return True


def get_md5_sum(filepath):
    """
    calculates md5 for a given file
    :param filepath: string containing path to file
    :return: string with md5sum
    """
    # read blocksize bytes at a time
    blocksize = 65536
    hasher = hashlib.md5()
    with open(filepath, 'rb') as afile:
        buf = afile.read(blocksize)
        while len(buf) > 0:
            hasher.update(buf)
            buf = afile.read(blocksize)
    return hasher.hexdigest()


def setup_dir_with_tmp_files():
    """
    Creates a tmp dir and populate it with some files and directories
    :return: tuple consisting of directory path and then a dict in the form {"path_item": type} where type is one of
            ["file", "dir"]
    """
    tmpdir = tempfile.mkdtemp(prefix="integration_test_")
    # tmpdir gets prepended to each item
    filelist = {
        tmpdir + os.sep + "dir1": "dir",
        tmpdir + os.sep + "dir1" + os.sep + "dir2": "dir",
        tmpdir + os.sep + "dir1" + os.sep + "dir2" + os.sep + "file1.txt": "file",
        tmpdir + os.sep + "dir1" + os.sep + "dir2" + os.sep + "file2⻆⽄〄㉎㍌㨂侣.html": "file",
        tmpdir + os.sep + "dir1" + os.sep + "dir2" + os.sep + "file3.txሥሿ": "file",
        tmpdir + os.sep + "dir1" + os.sep + "dir3ͲᾆЎ": "dir",
        tmpdir + os.sep + "dir1" + os.sep + "dir3ͲᾆЎ" + os.sep + "file4.html": "file",
        tmpdir + os.sep + "dir1" + os.sep + "dir3ͲᾆЎ" + os.sep + "file5صقڜ.txt": "file",
        tmpdir + os.sep + "dir1" + os.sep + "dir3ͲᾆЎ" + os.sep + "file6א.htmڿ": "file",
        tmpdir + os.sep + "dir1" + os.sep + "dir5": "dir",
        tmpdir + os.sep + "dir1" + os.sep + "dir5" + os.sep + "file7.txt": "file",
        tmpdir + os.sep + "dir1" + os.sep + "dir5" + os.sep + "file8.htm": "file",
        tmpdir + os.sep + "dir1" + os.sep + ";=&'file9.txt": "file",
    }

    for fname in filelist:
        ftype = filelist[fname]
        if ftype == "dir":
            os.makedirs(fname, exist_ok=True)
        elif ftype == "file":
            parent_dir = os.path.dirname(fname)
            if not os.path.exists(parent_dir):
                os.makedirs(parent_dir, exist_ok=True)
            with open(fname, "w", encoding="utf-8") as f:
                f.write("some text for " + fname)
    return tmpdir, filelist


# sets up a server config file to be used for various tests
# returns: path to config file; array of paths to delete (config file path, various temporary directories which may
# be needed)
def setup_tmp_config_file_and_tmp_dirs(suffix, config_file_content=working_server_config_file_content,
                                       persistent_null_store=False):
    """
    Write a temporary server config (derived from $config_file_content) with its own data_dir.

    :param persistent_null_store: when True every test_null target in the config gets a 'persist_dir'
        parameter pointing at a fresh temp directory. The daemon builds a new object store per job, so
        without it a restore cannot read back what an earlier backup job uploaded. Tests that restore, or
        that run the same job more than once, need this.
    :return: tuple of (config file path, list of paths to delete in tearDown). The data_dir is always
        element [1] of that list; the persist dir (if requested) is element [2].
    """
    tmphandle, config_file_path = tempfile.mkstemp(suffix=suffix + '__config.yaml')
    data_dir = tempfile.mkdtemp(suffix=suffix + '__datadir')
    server_config = config_file_content.replace("data_dir: ./tmp/", "data_dir: " + data_dir, 1)
    # ensure windows compatible paths are in the config on MS Windows
    if platform.system().lower() == 'windows':
        server_config = server_config.replace("- /something", r'- c:\something', 1)
        server_config = server_config.replace("- /var/lib", r'- c:\Windows\system', 1)
        server_config = server_config.replace("- /var/log", r'- c:\Program Files', 1)
        server_config = server_config.replace("- /var/www/html/data/", r'- C:\Windows\system32', 1)
    tmpfile = os.fdopen(tmphandle, "w")
    tmpfile.write(server_config)
    tmpfile.close()
    to_delete = [config_file_path, data_dir]
    if persistent_null_store:
        persist_dir = tempfile.mkdtemp(suffix=suffix + '__nullstore')
        add_persist_dir_to_null_targets(config_file_path, persist_dir)
        to_delete.append(persist_dir)
    return config_file_path, to_delete


def add_persist_dir_to_null_targets(config_file_path, persist_dir):
    """
    Rewrite the YAML config at $config_file_path so every target of type test_null carries a
    'persist_dir' parameter set to $persist_dir. Targets of a job share the directory: object keys
    include the target prefix and job name so they cannot collide.
    """
    with open(config_file_path) as fd:
        parsed = yaml.load(fd, Loader=yaml.SafeLoader)
    for job in parsed.get('backup', []):
        for target in job.get('target', []):
            if target.get('type') != 'test_null':
                continue
            params = [p for p in (target.get('parameters') or []) if p.get('name', '').lower() != 'persist_dir']
            params.append({'name': 'persist_dir', 'value': persist_dir})
            target['parameters'] = params
    with open(config_file_path, "w") as fd:
        fd.write(yaml.dump(parsed))


class CustomSMTPHandler:
    def __init__(self):
        self.received_messages = []

    async def handle_DATA(self, server, session, envelope):
        peer = session.peer
        mail_from = envelope.mail_from
        rcpt_tos = envelope.rcpt_tos
        data = envelope.content         # type: bytes
        logging.debug(f"Received via SMTP email from peer: {peer} with from: {mail_from} rcpt_tos: {rcpt_tos} and data: {data} ")
        # Process message data...
        # if error_occurred:
        #     return '500 Could not process your message'
        self.received_messages.append(data)
        return '250 OK'

    # helper methods for assertions in test cases
    def received_message_matching(self, template):
        for message in self.received_messages:
            decoded_quoted_printable = quopri.decodestring(message)
            decoded = decoded_quoted_printable.decode('utf-8', errors='replace')
            if re.search(template, decoded):
                return True, decoded
        return False, decoded

    def received_messages_count(self):
        return len(self.received_messages)


def start_smtp_controller(handler, port=None, ready_timeout=30):
    """
    Start an aiosmtpd SMTP server for handler on $port (a free port when None; read it back from
    controller.smtp_port). The aiosmtpd default ready timeout (5s) is too tight on a busy host
    (seen on macOS). If start() fails the controller is stopped before re-raising: setUp failing means tearDown won't
    run, so otherwise the server thread would keep the port bound and break every later test using it.
    """
    if port is None:
        port = find_free_port()
    controller = Controller(handler, hostname='localhost', port=port, ready_timeout=ready_timeout)
    controller.smtp_port = port
    try:
        controller.start()
    except Exception:
        controller.stop(no_assert=True)
        raise
    return controller

# mock smtp server, initial code taken from
# https://notepad.mmakowski.com/Tech/E-mail%20Testing%20with%20Mock%20SMTP%20Server
# class MockSMTPServer(smtpd.SMTPServer, threading.Thread):
#     '''
#     A mock SMTP server. Runs in a separate thread so can be started from
#     existing test code.
#     '''
#     def __init__(self, hostname, port):
#         self.socket_map = {}
#         threading.Thread.__init__(self)
#         smtpd.SMTPServer.__init__(self, (hostname, port), None, map=self.socket_map)
#         self.daemon = True
#         self.received_messages = []
#         self.start()
#
#     def run(self):
#         # put a really short timeout of 0.1 seconds (default is 30sec) as we want to exit as soon as possible when
#         # stopsmtpsrv() is called
#         asyncore.loop(timeout=0.1, map=self.socket_map)
#
#     # stop the smtp server
#     def stopsmtpsrv(self):
#         self.close()
#         self.join()
#
#     def process_message(self, peer, mailfrom, rcpttos, data, **kwargs):
#         self.received_messages.append(data)
#         return None
#
#     def reset(self):
#         self.received_messages = []
#
#     # helper methods for assertions in test cases
#     def received_message_matching(self, template):
#         for message in self.received_messages:
#             decoded_quoted_printable = quopri.decodestring(message)
#             decoded = decoded_quoted_printable.decode('utf-8', errors='replace')
#             if re.search(template, decoded):
#                 return True, decoded
#         return False, decoded
#
#     def received_messages_count(self):
#         return len(self.received_messages)


def get_s3_config_from_env():
    """
    Fetch from environment variables various settings needed by the AWS S3 object store
    :return: tuple with S3 bucket name (full virtualised mode name), S3 bucket region, AWS KEY ID, AWS SECRET .
    Any missing variables will have a value of None returned
    """
    bucket = os.environ.get('CLD_S3_BUCKET')
    aws_key = os.environ.get('CLD_AWS_ACCESS_KEY_ID')
    aws_secret = os.environ.get('CLD_AWS_SECRET_ACCESS_KEY')
    s3_region = os.environ.get('CLD_S3_REGION')
    return bucket, s3_region, aws_key, aws_secret


def get_gcp_storage_config_from_env():
    """
    Fetch from environment variables various settings needed by the GCP storage object store
    :return: tuple with S3 bucket name (full virtualised mode name), dict containing various credential related
                key+values(Any missing variables will have a value of None returned).
    """
    result = {}
    result["CLD_GCP_TYPE"] = os.environ.get("CLD_GCP_TYPE")
    result["CLD_GCP_PROJECT_ID"] = os.environ.get("CLD_GCP_PROJECT_ID")
    result["CLD_GCP_PRIVATE_KEY_ID"] = os.environ.get("CLD_GCP_PRIVATE_KEY_ID")
    tmp_key = os.environ.get("CLD_GCP_PRIVATE_KEY")
    if tmp_key:
        # remove literal "\n" and replace with newline character
        result["CLD_GCP_PRIVATE_KEY"] = tmp_key.replace("\\n", "\n")
    else:
        result["CLD_GCP_PRIVATE_KEY"] = tmp_key
    result["CLD_GCP_CLIENT_EMAIL"] = os.environ.get("CLD_GCP_CLIENT_EMAIL")
    result["CLD_GCP_CLIENT_ID"] = os.environ.get("CLD_GCP_CLIENT_ID")
    result["CLD_GCP_AUTH_URI"] = os.environ.get("CLD_GCP_AUTH_URI")
    result["CLD_GCP_TOKEN_URI"] = os.environ.get("CLD_GCP_TOKEN_URI")
    result["CLD_GCP_AUTH_PROVIDER_X509_CERT_URL"] = os.environ.get("CLD_GCP_AUTH_PROVIDER_X509_CERT_URL")
    result["CLD_GCP_CLIENT_X509_CERT_URL"] = os.environ.get("CLD_GCP_CLIENT_X509_CERT_URL")

    bucket = os.environ.get('CLD_GCP_STORAGE_BUCKET')

    return bucket, result


def get_azure_blob_storage_config_from_env():
    """
    Fetch from environment variables various settings needed by the Azure Blob Storage object store
    :return: tuple with Azure blob container name (bucket), Azure storage account name, Azure storage account key .
    Any missing variables will have a value of None returned
    """
    bucket = os.environ.get('CLD_AZURE_STORAGE_CONTAINER')
    azure_storage_account = os.environ.get('CLD_AZURE_STORAGE_ACCOUNT')
    azure_storage_account_key = os.environ.get('CLD_AZURE_STORAGE_ACCESS_KEY')

    return bucket, azure_storage_account, azure_storage_account_key


# counts the number of files, symlinks and directories in a given path (and all of the children of said path). If
#  $dereference==True then it follows symlinks (and will stop checking if a given item is a symlink or not) so if
#  $dereference==True then it will report 0 symlinks found no matter how many actual symlinks exist .
# Returns a touple with three elements: (files, directories, symlinks) examined
def count_files_folders_links(path, dereference=False):
    result_files = 0
    result_dirs = 0
    result_symlinks = 0
    if os.path.isdir(path):
        result_dirs = 1  # include top level dir too in result as the os.walk() function excludes it
        for root, dirs, files in os.walk(path, followlinks=dereference):
            for name in files:
                if dereference:
                    if os.path.islink(os.path.join(root, name)):
                        result_symlinks += 1
                    else:
                        result_files += 1
                else:
                    result_files += 1
            for _ in dirs:
                result_dirs += 1
    else:
        if dereference:
            result_files = 1
        else:
            if os.path.islink(path):
                result_symlinks += 1
            else:
                result_files += 1
    return result_files, result_dirs, result_symlinks


# fetches backup report and checks that various values match expectations
def check_backup_report(self, job_name, job_id, expected_num_files, expected_num_dirs, expected_num_symlinks):
    # get report of backup job and check that there were no errors and that the expected number of files got backed up
    logging.info("Getting report of backup job and checking it matches expectations")
    req = {"name": job_name,
           "job_id": job_id,
           }
    url = self.base_url + self.api_root + '/report/backup/show'
    r = requests.post(url=url, auth=(self.username, self.password), json=req)
    self.assertEqual(r.status_code, 200, url + " " + r.text)
    response = self.ValidatedAndDecodeResponse(r, url)
    # check response has expected keys
    self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                      " {}".format(url, r.text))
    self.assertEqual(response['code'], "success", "For {} response['code'] doesn't equal 'success'. Response was:"
                                                  " {}".format(url, r.text))
    self.assertEqual(response['message'], "success", "For {} response['message'] doesn't equal 'success'. Response"
                                                     " was: {}".format(url, r.text))
    self.assertEqual(response['result']['job_id'], job_id, "job_id in report is {} but we were expecting it to be"
                                                           " {}".format(response['result']['job_id'], job_id))
    self.assertEqual(response['result']['name'], job_name, "job_name in report is {} but we were expecting it to "
                                                           "be {}".format(response['result']['name'], job_name))
    self.assertEqual(response['result']['state'], "finished", "expected job state to be 'finished' but instead the"
                                                              " report shows it "
                                                              "as {}".format(response['result']['state']))
    self.assertEqual(response['result']['stats_counters']['examined_files'], expected_num_files,
                     "expected number of files to have been examined is {} but the report "
                     "shows {}".format(expected_num_files, response['result']['stats_counters']['examined_files']))
    self.assertEqual(response['result']['stats_counters']['examined_symlinks'], expected_num_symlinks,
                     "expected number of symlinks to have been examined is {} but the report "
                     "shows {}".format(expected_num_symlinks,
                                       response['result']['stats_counters']['examined_symlinks']))
    self.assertEqual(response['result']['stats_counters']['examined_directories'], expected_num_dirs,
                     "expected number of dirs to have been examined is {} but the report "
                     "shows {}".format(expected_num_dirs,
                                       response['result']['stats_counters']['examined_directories']))

    self.assertEqual(response['result']['stats_counters']['uploaded_files'], expected_num_files,
                     "expected number of files to have been uploaded is {} but the report "
                     "shows {}".format(expected_num_files, response['result']['stats_counters']['uploaded_files']))
    self.assertEqual(response['result']['stats_counters']['uploaded_symlinks'], expected_num_symlinks,
                     "expected number of symlinks to have been uploaded is {} but the report "
                     "shows {}".format(expected_num_symlinks,
                                       response['result']['stats_counters']['uploaded_symlinks']))
    self.assertEqual(response['result']['stats_counters']['uploaded_directories'], expected_num_dirs,
                     "expected number of dirs to have been uploaded is {} but the report "
                     "shows {}".format(expected_num_dirs,
                                       response['result']['stats_counters']['uploaded_directories']))
    for counter in ["database_copy_errors", "examined_unknown", "excluded", "failed_to_enumerate",
                    "failed_to_examine", "failed_to_find_deleted", "failed_to_mark_deleted_directories",
                    "failed_to_mark_deleted_files", "failed_to_mark_deleted_symlinks",
                    "failed_to_update_metadata_for_directories", "failed_to_update_metadata_for_files",
                    "failed_to_update_metadata_for_symlinks", "failed_to_upload_directories",
                    "failed_to_upload_files", "failed_to_upload_symlinks", "failed_to_upload_unknown",
                    "marked_deleted_directories", "marked_deleted_files", "marked_deleted_symlinks",
                    "scripts_failed", "scripts_num", "scripts_ran", "up_to_date_directories",
                    "up_to_date_files", "up_to_date_symlinks", "updated_metadata_for_directories",
                    "updated_metadata_for_files", "updated_metadata_for_symlinks"]:
        self.assertEqual(response['result']['stats_counters'][counter], 0,
                         "expected value of counter named '{}' in the backup report was 0 but instead {} was "
                         "found".format(counter, response['result']['stats_counters'][counter]))


# fetches restore report via /report/restore/show and checks that the restore counters match expectations
def check_restore_report(self, job_name, restore_job_id, expected_num_files, expected_num_dirs,
                         expected_num_symlinks):
    logging.info("Getting report of restore job and checking it matches expectations")
    req = {"name": job_name,
           "job_id": restore_job_id,
           }
    url = self.base_url + self.api_root + '/report/restore/show'
    r = requests.post(url=url, auth=(self.username, self.password), json=req)
    self.assertEqual(r.status_code, 200, url + " " + r.text)
    response = self.ValidatedAndDecodeResponse(r, url)
    self.assertIn("result", response, "Response for {} is missing the 'result' key. Response was:"
                                      " {}".format(url, r.text))
    self.assertEqual(response['code'], "success", "For {} response['code'] doesn't equal 'success'. Response was:"
                                                  " {}".format(url, r.text))
    self.assertEqual(response['message'], "success", "For {} response['message'] doesn't equal 'success'. Response"
                                                     " was: {}".format(url, r.text))
    self.assertEqual(response['result']['job_id'], restore_job_id, "job_id in restore report is {} but we were "
                                                                   "expecting it to be {}".format(
                                                                       response['result']['job_id'], restore_job_id))
    self.assertEqual(response['result']['name'], job_name, "name in restore report is {} but we were expecting it "
                                                           "to be {}".format(response['result']['name'], job_name))
    self.assertEqual(response['result']['state'], "finished", "expected restore state to be 'finished' but the "
                                                              "report shows it as {}".format(
                                                                  response['result']['state']))
    self.assertEqual(response['result']['stats_counters']['restored_files'], expected_num_files,
                     "expected number of files to have been restored is {} but the report "
                     "shows {}".format(expected_num_files,
                                       response['result']['stats_counters']['restored_files']))
    self.assertEqual(response['result']['stats_counters']['restored_directories'], expected_num_dirs,
                     "expected number of directories to have been restored is {} but the report "
                     "shows {}".format(expected_num_dirs,
                                       response['result']['stats_counters']['restored_directories']))
    self.assertEqual(response['result']['stats_counters']['restored_symlinks'], expected_num_symlinks,
                     "expected number of symlinks to have been restored is {} but the report "
                     "shows {}".format(expected_num_symlinks,
                                       response['result']['stats_counters']['restored_symlinks']))
    for counter in ["failed_to_restore_files", "skipped_delete_markers"]:
        self.assertEqual(response['result']['stats_counters'][counter], 0,
                         "expected value of counter named '{}' in the restore report was 0 but instead {} was "
                         "found".format(counter, response['result']['stats_counters'][counter]))


def make_inttest_logfile(prefix="integration_test_log_"):
    """
    Create a temporary log file for the daemon's --logfile option and return its path.

    tempfile.mkstemp() returns an *open* OS file descriptor; if it is left dangling
    the test process keeps a handle on the file. On Windows that handle blocks the
    teardown from deleting the file (it is then opened a second time by the daemon),
    so we close it immediately and let the daemon reopen the path by name.
    """
    fd, path = tempfile.mkstemp(prefix=prefix)
    os.close(fd)
    return path


def remove_file_with_retries(path, max_count=50, sleep_seconds=0.1):
    """
    Delete a file, tolerating the brief window on Windows where the file is still
    reported as in use by another process (WinError 32) right after the daemon that
    held its --logfile open has been killed. The OS releases the handle moments after
    the process dies, so we retry for a few seconds. If the file cannot be removed in
    time we log a warning rather than fail an otherwise-passing test's teardown.
    """
    if not os.path.exists(path):
        return
    for _ in range(max_count):
        try:
            os.remove(path)
            return
        except FileNotFoundError:
            return
        except PermissionError:
            time.sleep(sleep_seconds)
    logging.warning("Could not remove file '{}' after {} attempts; leaving it behind".format(path, max_count))


def map_path_into_restore_dir(restore_dir, source_path):
    """
    Mirror the server's mapPathIntoRestoreDir(): compute where an absolute source path
    lands once it has been restored under restore_dir. On Windows "C:\\foo\\bar" becomes
    "<restore_dir>\\C\\foo\\bar" (the drive-letter colon is dropped and the letter becomes
    a path component); on Unix "/foo/bar" becomes "<restore_dir>/foo/bar".

    The test must use the exact same mapping as the server. A naive
    os.path.join(restore_dir, source_path.lstrip(os.sep)) is correct on Unix but wrong on
    Windows: lstrip does not remove the drive letter, and os.path.join then discards
    restore_dir entirely (because the source path is still drive-absolute), so the test
    would silently check the original source location instead of the restored copy.
    """
    clean = os.path.normpath(source_path)
    if platform.system() == 'Windows':
        drive, rest = os.path.splitdrive(clean)
        rest = rest.lstrip("\\/")
        if drive:
            return os.path.join(restore_dir, drive[0], rest)
        return os.path.join(restore_dir, rest)
    return os.path.join(restore_dir, clean.lstrip("/"))


def verify_restored_tree(self, restore_dir, source_root, filelist, dereference=False, absent=None):
    """
    Assert that every entry of $filelist ({path: "file"|"dir"}) was restored under $restore_dir with the
    server's path mapping, that regular files carry the same md5 as the source, and that the restored
    subtree holds exactly as many files/dirs/symlinks as the source tree at $source_root.
    :param absent: optional iterable of source paths that must NOT exist under restore_dir
    :return: (num_files, num_dirs, num_symlinks) counted on the source side, handy for check_restore_report()
    """
    for source_path, file_type in filelist.items():
        restored_path = map_path_into_restore_dir(restore_dir, source_path)
        self.assertTrue(os.path.exists(restored_path),
                        "Expected restored item '{}' (type={}) to exist at '{}' but it does "
                        "not".format(source_path, file_type, restored_path))
        if file_type == "dir":
            self.assertTrue(os.path.isdir(restored_path), "Expected '{}' to be a directory".format(restored_path))
        elif file_type == "file":
            self.assertTrue(os.path.isfile(restored_path), "Expected '{}' to be a regular file".format(restored_path))
            original_md5 = get_md5_sum(source_path)
            restored_md5 = get_md5_sum(restored_path)
            self.assertEqual(original_md5, restored_md5,
                             "MD5 mismatch for '{}': original={} restored={}".format(
                                 source_path, original_md5, restored_md5))
    for source_path in (absent or []):
        restored_path = map_path_into_restore_dir(restore_dir, source_path)
        self.assertFalse(os.path.exists(restored_path),
                         "'{}' was not requested but was restored at '{}'".format(source_path, restored_path))
    expected = count_files_folders_links(source_root, dereference)
    restored_root = map_path_into_restore_dir(restore_dir, source_root)
    restored = count_files_folders_links(restored_root, dereference)
    self.assertEqual(restored, expected,
                     "restored tree at '{}' has (files, dirs, symlinks)={} but the source at '{}' has {}".format(
                         restored_root, restored, source_root, expected))
    return expected


class ApiClient(object):
    """
    Thin REST client used by the pytest fixtures: validates the response envelope every call, raises
    AssertionError with the URL and body on failure, and carries the wait/poll helpers that the older
    unittest modules each re-implement.
    """
    def __init__(self, base_url, username='testuser1', password='HV}H/y?<9$]Z5N4N', api_root='/api/v1'):
        self.base_url = base_url
        self.username = username
        self.password = password
        self.auth = (username, password)
        self.api_root = api_root

    # ---- unittest-style assertion surface, so check_backup_report(), check_restore_report() and
    # verify_restored_tree() (written for TestCase "self") can be handed an ApiClient from pytest tests
    def fail(self, msg):
        raise AssertionError(msg)

    def assertTrue(self, expr, msg=None):
        assert expr, msg or "expected a true value, got {!r}".format(expr)

    def assertFalse(self, expr, msg=None):
        assert not expr, msg or "expected a false value, got {!r}".format(expr)

    def assertEqual(self, a, b, msg=None):
        assert a == b, msg or "{!r} != {!r}".format(a, b)

    def assertNotEqual(self, a, b, msg=None):
        assert a != b, msg or "{!r} == {!r}".format(a, b)

    def assertIn(self, member, container, msg=None):
        assert member in container, msg or "{!r} not found in {!r}".format(member, container)

    def assertNotIn(self, member, container, msg=None):
        assert member not in container, msg or "{!r} unexpectedly found in {!r}".format(member, container)

    def assertGreater(self, a, b, msg=None):
        assert a > b, msg or "{!r} not greater than {!r}".format(a, b)

    def assertIsNotNone(self, obj, msg=None):
        assert obj is not None, msg or "unexpectedly None"

    def ValidatedAndDecodeResponse(self, r, url):
        return self._validated(r, url, r.status_code)

    def url(self, path):
        return self.base_url + self.api_root + path

    def get(self, path, expect=200, **kw):
        r = requests.get(url=self.url(path), auth=self.auth, **kw)
        return self._validated(r, path, expect)

    def post(self, path, body=None, expect=200, **kw):
        r = requests.post(url=self.url(path), auth=self.auth, json=body, **kw)
        return self._validated(r, path, expect)

    def raw_post(self, path, body=None, **kw):
        return requests.post(url=self.url(path), auth=self.auth, json=body, **kw)

    def raw_get(self, path, **kw):
        return requests.get(url=self.url(path), auth=self.auth, **kw)

    def _validated(self, r, path, expect):
        assert r.status_code == expect, "{} {} -> {}: {}".format(r.request.method, path, r.status_code, r.text)
        ctype = r.headers.get('Content-Type', '')
        assert ctype == 'application/json', "{} has Content-Type '{}' instead of application/json".format(path, ctype)
        body = r.json()
        for key in ("code", "message"):
            assert key in body, "{} response is missing '{}': {}".format(path, key, r.text)
        return body

    # ---- job helpers
    def start_backup(self, job_name):
        return self.post('/backup/start', {"name": job_name})['result']['job_id']

    def backup_state(self, job_name):
        for backup in self.get('/backup/list')['result']:
            if backup['name'] == job_name:
                return backup['state']
        raise AssertionError("backup job '{}' not present in /backup/list".format(job_name))

    def wait_backup_finishes(self, job_name, max_seconds=60):
        deadline = time.time() + max_seconds
        while time.time() < deadline:
            if self.backup_state(job_name) == 'stopped':
                return
            time.sleep(0.1)
        raise AssertionError("Backup '{}' did not finish in {} seconds".format(job_name, max_seconds))

    def run_backup_and_wait(self, job_name, max_seconds=60):
        job_id = self.start_backup(job_name)
        self.wait_backup_finishes(job_name, max_seconds)
        return job_id

    def backup_report(self, job_name, job_id):
        return self.post('/report/backup/show', {"name": job_name, "job_id": job_id})['result']

    def start_restore(self, job_name, source_backup_job_id, restore_dir, files=None, all_files=False, **extra):
        body = {"name": job_name, "source_backup_job_id": source_backup_job_id, "restore_dir": restore_dir}
        if all_files:
            body["all_files"] = True
        if files is not None:
            body["files"] = files
        body.update(extra)
        return self.post('/restore/start', body)['result']['restore_job_id']

    def restore_running(self, job_name, restore_job_id):
        for entry in self.get('/restore/list')['result']:
            if entry['name'] == job_name and entry.get('job_id', '') == restore_job_id:
                return True
        return False

    def wait_restore_finishes(self, job_name, restore_job_id, max_seconds=60):
        deadline = time.time() + max_seconds
        while time.time() < deadline:
            if not self.restore_running(job_name, restore_job_id):
                return
            time.sleep(0.1)
        raise AssertionError("Restore '{}' (restore_job_id={}) did not finish in {} seconds".format(
            job_name, restore_job_id, max_seconds))

    def restore_report(self, job_name, restore_job_id):
        return self.post('/report/restore/show', {"name": job_name, "job_id": restore_job_id})['result']

    def set_target_ratelimit(self, job_name, ratelimit, target_index=0):
        """Slow the (instant) test_null transfers down so a running job can be observed."""
        config = self.get('/config')['result']
        for job in config['backup']:
            if job['name'] == job_name:
                job['target'][target_index]['ratelimit'] = ratelimit
                break
        else:
            raise AssertionError("backup job '{}' not found in /config".format(job_name))
        self.post('/config', config)


class CliResult(object):
    """Outcome of one cloudbackup CLI invocation."""
    def __init__(self, cmdline, completed):
        self.cmdline = cmdline
        self.returncode = completed.returncode
        self.stdout = completed.stdout.decode("utf-8", errors="replace")
        self.stderr = completed.stderr.decode("utf-8", errors="replace")

    def __repr__(self):
        return "CliResult(cmd={!r}, rc={}, stdout={!r}, stderr={!r})".format(
            self.cmdline, self.returncode, self.stdout, self.stderr)

    @property
    def lines(self):
        return [ln for ln in self.stdout.split('\n') if ln != '']

    def json(self):
        return json.loads(self.stdout)


def run_cli(args, client_config=None, expect=0, cmd=None):
    """
    Run "./cloudbackup <args>" (a string, shell-quoted by the caller where needed). When $client_config is
    given, " -c <path>" is appended. Asserts the exit code equals $expect (None to skip the check).
    """
    cmdline = (cmd or cmd_default) + " " + args
    if client_config:
        cmdline += " -c " + client_config
    result = CliResult(cmdline, run_shell_cmd(cmdline)['result'])
    if expect is not None:
        assert result.returncode == expect, "exit code {} (expected {}) from: {}".format(
            result.returncode, expect, result)
    return result


def verify_restored_subset(self, restore_dir, filelist, requested):
    """
    After a restore that asked only for $requested (absolute source paths, files or directories), assert
    that every filelist entry inside one of them was restored (md5-checked for files) and that nothing
    outside was, ancestors excepted: MkdirAll creates parent directories as a side effect.
    """
    wanted = [r.rstrip(os.sep) for r in requested]
    for source_path, file_type in filelist.items():
        restored_path = map_path_into_restore_dir(restore_dir, source_path)
        inside = any(source_path == w or source_path.startswith(w + os.sep) for w in wanted)
        ancestor = any(w.startswith(source_path + os.sep) for w in wanted)
        if inside:
            self.assertTrue(os.path.exists(restored_path),
                            "Expected restored item '{}' (type={}) at '{}'".format(source_path, file_type, restored_path))
            if file_type == "file":
                self.assertEqual(get_md5_sum(source_path), get_md5_sum(restored_path),
                                 "MD5 mismatch for restored file '{}'".format(source_path))
        elif not ancestor:
            self.assertFalse(os.path.exists(restored_path),
                             "'{}' is outside the requested paths {} but was restored at '{}'".format(
                                 source_path, requested, restored_path))


def assert_counters(self, stats, expected, context=""):
    """Assert that every counter in $expected ({name: value}) has that value in $stats (a stats_counters dict)."""
    for name, value in expected.items():
        self.assertIn(name, stats, "{}counter '{}' missing from stats: {}".format(context, name, stats))
        self.assertEqual(stats[name], value, "{}counter '{}' is {} but {} was expected. All counters: {}".format(
            context, name, stats[name], value, stats))


def make_self_signed_cert(directory, ip='127.0.0.1'):
    """
    Generate a self-signed certificate + key for $ip (as a subjectAltName, which Go requires) with the openssl
    CLI. Returns (cert_path, key_path) or None when openssl is unavailable.
    """
    if shutil.which('openssl') is None:
        return None
    cert = os.path.join(directory, 'server.crt')
    key = os.path.join(directory, 'server.key')
    cmd = ['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2', '-keyout', key, '-out', cert,
           '-subj', '/CN={}'.format(ip), '-addext', 'subjectAltName=IP:{}'.format(ip)]
    result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode != 0:
        raise RuntimeError("openssl failed: {}".format(result.stderr.decode('utf-8', errors='replace')))
    return cert, key
