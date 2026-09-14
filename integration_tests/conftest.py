"""
pytest wiring shared by every tier.

- puts integration_tests/lib on sys.path so test modules keep their historical "from common import *"
- provides fixtures for tests written in function style; the older unittest.TestCase modules keep their
  own setUp()/tearDown() and simply ignore these
"""
import os
import shutil
import sys

import pytest

_LIB = os.path.join(os.path.dirname(os.path.abspath(__file__)), "lib")
if _LIB not in sys.path:
    sys.path.insert(0, _LIB)

import yaml  # noqa: E402

from common import (  # noqa: E402
    ApiClient,
    BackupDaemon,
    make_inttest_logfile,
    remove_file_with_retries,
    setup_dir_with_tmp_files,
    setup_tmp_config_file_and_tmp_dirs,
    write_client_config,
)


class SourceTree(object):
    """A temporary directory populated by setup_dir_with_tmp_files()."""
    def __init__(self, root, filelist):
        self.root = root
        self.filelist = filelist

    @property
    def num_files(self):
        return sum(1 for t in self.filelist.values() if t == "file")

    @property
    def num_dirs(self):
        return sum(1 for t in self.filelist.values() if t == "dir")


class ServerConfig(object):
    """Path of a temporary server config plus the temp dirs it references."""
    def __init__(self, path, to_delete):
        self.path = path
        self.to_delete = to_delete
        self.data_dir = to_delete[1]
        self.persist_dir = to_delete[2] if len(to_delete) > 2 else None

    def load(self):
        with open(self.path) as fd:
            return yaml.load(fd, Loader=yaml.SafeLoader)

    def save(self, parsed):
        with open(self.path, "w") as fd:
            fd.write(yaml.dump(parsed))

    def patch(self, fn):
        """Apply fn(parsed_config) and write the result back."""
        parsed = self.load()
        fn(parsed)
        self.save(parsed)


@pytest.fixture
def source_tree():
    root, filelist = setup_dir_with_tmp_files()
    yield SourceTree(root, filelist)
    shutil.rmtree(root, ignore_errors=True)


@pytest.fixture
def server_config(request, source_tree):
    """
    Temporary server config whose first_backup job backs up $source_tree through a persistent test_null
    target. Tests tweak it further with server_config.patch(...) before requesting the daemon fixture.
    """
    path, to_delete = setup_tmp_config_file_and_tmp_dirs(suffix="_" + request.node.name[:40],
                                                         persistent_null_store=True)
    cfg = ServerConfig(path, to_delete)

    def point_first_job_at_tree(parsed):
        parsed["backup"][0]["paths"] = [source_tree.root]
        parsed["backup"][0]["exclusions"] = [""]
    cfg.patch(point_first_job_at_tree)
    yield cfg
    for entry in to_delete:
        if os.path.isdir(entry):
            shutil.rmtree(entry, ignore_errors=True)
        elif os.path.exists(entry):
            os.remove(entry)


@pytest.fixture
def daemon(server_config):
    """A running daemon on a free port. daemon.base_url is where it listens."""
    logfile = make_inttest_logfile()
    d = BackupDaemon(config_path=server_config.path, extra_options="--logfile=" + logfile)
    yield d
    d.kill()
    remove_file_with_retries(logfile)


@pytest.fixture
def client_config(daemon):
    """Path of a client config (testuser1, write access) pointing at $daemon."""
    path = write_client_config(daemon.base_url)
    yield path
    if os.path.exists(path):
        os.remove(path)


@pytest.fixture
def api(daemon):
    """ApiClient authenticated as testuser1 (write access) against $daemon."""
    return ApiClient(daemon.base_url)


@pytest.fixture
def api_readonly(daemon):
    """ApiClient authenticated as testuser2 (read-only access) against $daemon."""
    return ApiClient(daemon.base_url, username="testuser2", password="Oonaawai8Eep]eethe8eefa$")


@pytest.fixture
def restore_dir():
    import tempfile
    path = tempfile.mkdtemp(prefix="integration_test_restore_dest_")
    yield path
    shutil.rmtree(path, ignore_errors=True)
