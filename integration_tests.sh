#!/bin/sh

export PYTHONIOENCODING='utf-8'
export TESTSFOLDER='./integration_tests'

if [ ! -x ${TESTSFOLDER}/.venv/bin/python ]; then
  echo "Setting up virtualenv for Python integration tests ..."
  virtualenv -p python3 ${TESTSFOLDER}/.venv
  if [ $? -ne 0 ]; then
    echo 'Error setting up the virtualenv'
    exit 1
  fi
fi
echo "Installing dependencies needed for Python integration tests ..."
${TESTSFOLDER}/.venv/bin/pip install -q -r ${TESTSFOLDER}/requirements.txt
if [ $? -ne 0 ]; then
  echo "Error installing required python packages in the virtualenv"
  exit 1
fi

# list installed depencencies and the versions of their dependencies
${TESTSFOLDER}/.venv/bin/python --version
${TESTSFOLDER}/.venv/bin/pip freeze

echo "Linting Python integration tests ..."
# We put the linting here for simplicity, since this is not a Python project
${TESTSFOLDER}/.venv/bin/flake8 --ignore E501,F401,F403,F405,W504,W605 ${TESTSFOLDER}/ --exclude=.venv
if [ $? -ne 0 ]; then
  echo 'Linting error'
  exit 1
fi

echo "Running Python integration tests ..."
${TESTSFOLDER}/.venv/bin/python -m unittest discover -s ${TESTSFOLDER}/ -p '*.py*' -v
if [ $? -ne 0 ]; then
  echo 'Test error'
  exit 1
fi

echo "Cleaning up object stores as the test is complete ..."
${TESTSFOLDER}/.venv/bin/python ${TESTSFOLDER}/clean_object_stores_after_tests.py
if [ $? -ne 0 ]; then
  echo 'Post test cleanup error'
  exit 1
fi
