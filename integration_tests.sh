#!/bin/sh

set -x
export PYTHONIOENCODING='utf-8'
export TESTSFOLDER='./integration_tests'

if [ ! -x ${TESTSFOLDER}/.venv/bin/python ]; then
  echo "Setting up virtualenv for Python unit tests ..."
  virtualenv -p python3 ${TESTSFOLDER}/.venv
  if [ $? -ne 0 ]; then
    echo 'Error setting up the virtualenv'
    exit 1
  fi
fi
echo "Installing dependencies needed for Python unit tests ..."
${TESTSFOLDER}/.venv/bin/pip install -q -r ${TESTSFOLDER}/requirements.txt
if [ $? -ne 0 ]; then
  echo "Error installing required python packages in the virtualenv"
  exit 1
fi

echo "Linting Python unit tests ..."
# We put the linting here for simplicity, since this is not a Python project
${TESTSFOLDER}/.venv/bin/flake8 --ignore E501,F401,F403,F405 ${TESTSFOLDER}/ --exclude=.venv
if [ $? -ne 0 ]; then
  echo 'Linting error'
  exit 1
fi

echo "Running Python unit tests ..."
${TESTSFOLDER}/.venv/bin/python -m unittest discover -s ${TESTSFOLDER}/ -p '*.py*' 
