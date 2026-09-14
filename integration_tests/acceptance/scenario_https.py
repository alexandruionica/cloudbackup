#!/usr/bin/env python
"""
Scenario for user guide 3 ("https:") and 5: the daemon serves the API over TLS with an operator-supplied
certificate, plain HTTP is off, and the CLI client connects to an https:// address, verifying the
certificate. The client trusts the system store, so the self-signed test certificate is handed to it via
SSL_CERT_FILE, which Go honours on Linux and FreeBSD only.
"""
import os
import platform
import shlex
import tempfile

import pytest

from common import *


@pytest.fixture
def tls_material():
    directory = tempfile.mkdtemp(prefix="integration_test_tls_")
    material = make_self_signed_cert(directory)
    if material is None:
        pytest.skip("openssl CLI not available to mint a test certificate")
    yield material
    import shutil
    shutil.rmtree(directory, ignore_errors=True)


@pytest.mark.skipif(platform.system() not in ("Linux", "FreeBSD"),
                    reason="the client trusts the OS store; SSL_CERT_FILE is only honoured by Go on Linux/FreeBSD")
def test_client_talks_to_daemon_over_https(server_config, source_tree, tls_material):
    cert, key = tls_material

    def enable_https(parsed):
        parsed["https"] = {"enabled": True, "ssl_cert_path": cert, "ssl_key_path": key}
    server_config.patch(enable_https)
    run_cli("server config validate -c " + shlex.quote(server_config.path))

    base_url = "https://127.0.0.1:{}".format(find_free_port())
    daemon = BackupDaemon(config_path=server_config.path, base_url=base_url, tls_ca=cert,
                          extra_options="--logfile=" + os.path.join(server_config.data_dir, "https.log"))
    try:
        client_config = write_client_config(base_url)
        env_with_ca = dict(os.environ, SSL_CERT_FILE=cert)

        # with the certificate trusted, the documented commands work over TLS
        with_ca = subprocess.run(cmd_default + " client server-version -c " + client_config, shell=True,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env_with_ca)
        assert with_ca.returncode == 0 and b"Server version:" in with_ca.stdout, with_ca
        listed = subprocess.run(cmd_default + " client backup list --json -c " + client_config, shell=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env_with_ca)
        assert listed.returncode == 0, listed
        assert {j["name"] for j in json.loads(listed.stdout)["result"]} == {"first_backup", "second_backup"}

        # without it the client refuses the connection: verification is on, not skipped
        without = run_cli("client server-version", client_config, expect=1)
        assert "certificate" in (without.stdout + without.stderr).lower(), without

        # plain HTTP on the TLS port is not an API
        with pytest.raises(requests.exceptions.RequestException):
            requests.get(base_url.replace("https://", "http://") + "/api/v1/backup/list", timeout=5).raise_for_status()
        os.remove(client_config)
    finally:
        daemon.kill()
