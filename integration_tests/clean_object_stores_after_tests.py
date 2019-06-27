import argparse
import boto3
import logging
import os
import platform


def clean_s3_bucket(prefix=None):
    bucket = os.environ.get('CLD_S3_BUCKET')
    if not bucket:
        logging.error("Environment variable CLD_S3_BUCKET is not set")
        exit(1)
    aws_key = os.environ.get('CLD_AWS_ACCESS_KEY_ID')
    aws_secret = os.environ.get('CLD_AWS_SECRET_ACCESS_KEY')

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

    # "tests/" + runtime.GOOS + "/"
    platform.system().lower()
    if prefix:
        final_prefix = prefix
    else:
        final_prefix = "tests/" + platform.system().lower() + "/"
    bucket.objects.filter(Prefix=final_prefix).delete()


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
    if arguments.verbose:
        logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.DEBUG)
    else:
        logging.basicConfig(format='%(levelname)s: %(message)s', level=logging.INFO)

    clean_s3_bucket(arguments.prefix)


if __name__ == '__main__':
    main()
