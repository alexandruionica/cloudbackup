import argparse
import boto3
import json
import logging
import os
import platform
from common import *
from google.cloud import storage
from azure.storage.blob import ContainerClient
from pprint import pprint


def clean_s3_bucket(prefix=None):
    bucket, _, aws_key, aws_secret = get_s3_config_from_env()
    if not bucket:
        logging.error("Environment variable CLD_S3_BUCKET is not set")
        exit(1)

    if aws_key and aws_secret:
        session = boto3.Session(
            aws_access_key_id=aws_key,
            aws_secret_access_key=aws_secret,
        )
        s3 = session.resource('s3')
    else:
        # use the default credentials search path
        s3 = boto3.resource('s3')

    bucket = s3.Bucket(bucket)

    # bucket.objects.filter(Prefix=final_prefix).delete()
    # delete all objects and all versions under the prefix
    bucket.object_versions.filter(Prefix=prefix).delete()


def clean_gcp_storage_bucket(prefix=None):
    bucket, settings = get_gcp_storage_config_from_env()
    if settings["CLD_GCP_PRIVATE_KEY"]:
        credentials = {
            "type": settings["CLD_GCP_TYPE"],
            "project_id": settings["CLD_GCP_PROJECT_ID"],
            "private_key_id": settings["CLD_GCP_PRIVATE_KEY_ID"],
            "private_key": settings["CLD_GCP_PRIVATE_KEY"],
            "client_email": settings["CLD_GCP_CLIENT_EMAIL"],
            "client_id": settings["CLD_GCP_CLIENT_ID"],
            "auth_uri": settings["CLD_GCP_AUTH_URI"],
            "token_uri": settings["CLD_GCP_TOKEN_URI"],
            "auth_provider_x509_cert_url": settings["CLD_GCP_AUTH_PROVIDER_X509_CERT_URL"],
            "client_x509_cert_url": settings["CLD_GCP_CLIENT_X509_CERT_URL"],
        }
        tmphandle, credentials_file_path = tempfile.mkstemp(suffix='_gcp_credentials.json')
        tmpfile = os.fdopen(tmphandle, "w")
        tmpfile.write(json.dumps(credentials, indent=2))
        tmpfile.close()
        # set env var GOOGLE_APPLICATION_CREDENTIALS as the SDK will search for this
        os.environ['GOOGLE_APPLICATION_CREDENTIALS'] = credentials_file_path
        logging.info("Credentials file is {}".format(credentials_file_path))

    storage_client = storage.Client()
    blobs = storage_client.list_blobs(bucket, prefix=prefix, versions=True)
    for blob in blobs:
        logging.info("Deleting GCP blob '{}'".format(blob.name))
        blob.delete()

    if settings["CLD_GCP_PRIVATE_KEY"]:
        if os.path.exists(credentials_file_path):
            os.remove(credentials_file_path)


def clean_azure_blob_container(prefix=None):
    bucket, azure_storage_account, azure_storage_account_key = get_azure_blob_storage_config_from_env()
    if not bucket:
        logging.error("Environment variable CLD_AZURE_STORAGE_CONTAINER is not set")
        exit(1)
    if not azure_storage_account:
        logging.error("Environment variable CLD_AZURE_STORAGE_ACCOUNT is not set")
        exit(1)
    if not azure_storage_account_key:
        logging.error("Environment variable CLD_AZURE_STORAGE_ACCESS_KEY is not set")
        exit(1)

    container_client = ContainerClient(
        account_url="https://{}.blob.core.windows.net".format(azure_storage_account),
        container_name=bucket,
        credential=azure_storage_account_key,
    )
    for blob in container_client.list_blobs(name_starts_with=prefix):
        logging.info("Deleting Azure blob '{}'".format(blob.name))
        container_client.delete_blob(blob.name)


def get_args():
    """ Get arguments from CLI """

    parser = argparse.ArgumentParser(description='Script which cleans up all object stores used for tests')
    parser.add_argument('-v', '--verbose', required=False, action="store_true", default=False,
                        help='Show verbose level messages')
    parser.add_argument('-p', '--prefix', required=False, default=None,
                        help='Prefix to delete from each object store. If not defined, then object store specific '
                             'defaults are used')
    args = parser.parse_args()
    return args


def main():
    arguments = get_args()
    # 'azure' covers both azure.storage and azure.core.* (the latter's
    # http_logging_policy emits a full request/response dump at INFO).
    azure_logger = logging.getLogger('azure')
    if arguments.verbose:
        logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.INFO)
        azure_logger.setLevel(logging.INFO)
    else:
        logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.WARNING)
        azure_logger.setLevel(logging.WARNING)

    if arguments.prefix:
        final_prefix = arguments.prefix
    else:
        final_prefix = "tests/" + platform.system().lower() + "/"

    clean_s3_bucket(final_prefix)

    clean_gcp_storage_bucket(final_prefix)

    clean_azure_blob_container(final_prefix)


if __name__ == '__main__':
    main()
