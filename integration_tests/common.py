#!/usr/bin/env python
import asyncore
import hashlib
import logging
import platform
import os
import re
import requests
import smtpd
import socket
import shlex
import subprocess
import threading
import time
import tempfile
import quopri


logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.INFO)

if platform.system().lower() == 'windows':
    cmd_default = ".\cloudbackup.exe"
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


class BackupDaemon(object):
    """
    Start cloudbackup daemon
    """
    def __init__(self, config_path, base_url, cmd=cmd_default, extra_options=""):
        """
        start backup daemon
        Wrapper to start a shell command which then keeps running
        :param cmd: command to run
        :param base_url: where the API server will be reachable. Example "http://127.0.0.1:8080"
        :return: { 'result': None/subprocess.CompletedProcess,
                   'exception: None/exception ..}
        """
        # check ip:port is available
        wait_for_socket(base_url)

        if platform.system() == 'Windows':
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
        if not check_api_server_ready(base_url):
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


def check_api_server_ready(url, max_count=20, sleep_seconds=0.1):
    """
    Attempt $max_count times, with $sleep_seconds seconds sleep to get / from the http server. This is to give time
       to start up
    :return: False if it did not start up during wait time, True if succeeded
    """
    counter = 0
    while counter < max_count:
        try:
            requests.get(url)
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
        tmpdir + os.sep + "dir1" + os.sep + ";<>file9.txt": "file",
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
def setup_tmp_config_file_and_tmp_dirs(suffix, config_file_content=working_server_config_file_content):
    tmphandle, config_file_path = tempfile.mkstemp(suffix=suffix + '__config.yaml')
    data_dir = tempfile.mkdtemp(suffix=suffix + '__datadir')
    server_config = config_file_content.replace("data_dir: ./tmp/", "data_dir: " + data_dir, 1)
    tmpfile = os.fdopen(tmphandle, "w")
    tmpfile.write(server_config)
    tmpfile.close()
    return config_file_path, [config_file_path, data_dir]


# mock smtp server, initial code taken from
# https://notepad.mmakowski.com/Tech/E-mail%20Testing%20with%20Mock%20SMTP%20Server
class MockSMTPServer(smtpd.SMTPServer, threading.Thread):
    '''
    A mock SMTP server. Runs in a separate thread so can be started from
    existing test code.
    '''
    def __init__(self, hostname, port):
        self.socket_map = {}
        threading.Thread.__init__(self)
        smtpd.SMTPServer.__init__(self, (hostname, port), None, map=self.socket_map)
        self.daemon = True
        self.received_messages = []
        self.start()

    def run(self):
        # put a really short timeout of 0.1 seconds (default is 30sec) as we want to exit as soon as possible when
        # stopsmtpsrv() is called
        asyncore.loop(timeout=0.1, map=self.socket_map)

    # stop the smtp server
    def stopsmtpsrv(self):
        self.close()
        self.join()

    def process_message(self, peer, mailfrom, rcpttos, data, **kwargs):
        self.received_messages.append(data)
        return None

    def reset(self):
        self.received_messages = []

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
