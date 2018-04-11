#!/usr/bin/env python
import hashlib
import logging
import platform
import requests
import os
import socket
import shlex
import subprocess
import time
from pprint import pprint

if platform.system() == 'Windows':
    cmd_default = ".\cloudbackup.exe"
else:
    cmd_default = "./cloudbackup"
working_config_file_content = '''# global settings affect all backups and can't be specified per backup with different values
# section specific settings are repetitive and can't be overridden by globals
# clarity and safety are paramount to the design so repeating a particular key - value over and over is acceptable
#
#
data_dir: ./tmp/
#defaults to webstatic/ relative to where the cloudbackup binary is located
html_dir: webstatic
user:
  - name: testuser1
    # bcrypt hash of password  "HV}H/y?<9$]Z5N4N" - use ./cloudbackup hash-password to hash passwords
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
        type: aws_s3
        user: BLABLA
        pass: zzzz
        bucket: 'myawesome-backup'
        prefix: 'backup/backups-for-server-51'
        storage_class: standard
    schedule:
      - '05 01 * * *'
  - name: second_backup
    paths:
      - /var/log
      - /var/www/html/data/
    target:
      - name: aws_2
        type: aws_s3
        user: JOHNDOE
        pass: qwqe
        bucket: 'some-stuff-goes-here'
        prefix: 'backup/backups-for-server-51'
        storage_class: 'infrequent-access'
      - name: google_1
        type: google_cloud_storage
        user: JANEDOE
        pass: 34324fd
        bucket: 'my-google-bucket'
        prefix: 'backup/backups-for-server-51'
        storage_class: standard
    encrypt: true
    encrypt_pass: '044ewfsoi423092l;dfksdl;fksl;dfks;ld0492'
    schedule:
      - '00 08 01 * *'
      - '00 08 06 * *'
    versioning: true
    versions_max_num: 10
    versions_max_age: 6w
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
    def __init__(self, config_path, base_url, cmd=cmd_default):
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
            full_cmd = os.path.abspath(cmd) + ' start -c {}'.format(os.path.abspath(config_path))
            cmd_with_args = full_cmd
        else:
            full_cmd = cmd + ' start -c {}'.format(config_path)
            cmd_with_args = shlex.split(cmd + ' start -c {}'.format(config_path))
        logging.info('Running the backup daemon using: {}'.format(full_cmd))
        self.proc = subprocess.Popen(cmd_with_args, shell=False, stdin=subprocess.PIPE,
                                     stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        # there is a slight delay between daemon start and http becoming available so we need to ensure it is
        #   available before tests are attempted
        wait_for_api_server(base_url)

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

    def stop(self, max_count=20, sleep_time=0.1):
        """
        stop daemon using terminate()
        :return: True on success, False if process already exited
        """
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
            # close file descriptors for stdin/stdout/stderr
            self.proc.stderr.close()
            self.proc.stdout.close()
            self.proc.stdin.close()
            return True
        else:
            return False

    def is_running(self):
        """
        check if daemon still running
        :return: True if running, False if exited
        """
        if self.proc.poll() is None:
            return True
        else:
            return False


def wait_for_socket(base_url, max_count=400, sleep_seconds=0.1):
    """
    Attempt 400 times, with 0.1 seconds sleep to bind on the listening IP:port. This is to give time for whatever keeps
    the port open to close it before we attempt to run the test
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


def wait_for_api_server(url, max_count=20):
    """
    Attempt 20 times, with 0.1 seconds sleep to get / from the http server. This is to give time to start up
    :return:
    """
    counter = 0
    while counter < max_count:
        try:
            requests.get(url)
        except requests.exceptions.ConnectionError:
            time.sleep(0.1)
            counter += 1
            continue
        else:
            counter = 0
            break
    if counter == max_count:
        raise requests.exceptions.ConnectionError(
            "Could not connect to CloudBackup API server at {} after {} attempts".format(url, counter))


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
