#!/bin/sh

export PYTHONIOENCODING='utf-8'
export TESTSFOLDER='./integration_tests'

# A virtualenv is not portable across operating systems: it hardcodes the
# interpreter path and holds platform-specific wheels. The same checkout is
# shared with the FreeBSD and macOS VMs over a synced folder, so give each OS
# its own directory rather than letting them fight over one.
case "$(uname -s)" in
  Linux)   VENV="${TESTSFOLDER}/.venv_linux" ;;
  Darwin)  VENV="${TESTSFOLDER}/.venv_macos" ;;
  FreeBSD) VENV="${TESTSFOLDER}/.venv_freebsd" ;;
  *)
    echo "Unsupported OS for the integration tests: $(uname -s)" >&2
    echo "Windows runs them through integration_tests.ps1 instead." >&2
    exit 1
    ;;
esac
echo "Using virtualenv ${VENV}"

if [ ! -x ${VENV}/bin/python ]; then
  echo "Setting up virtualenv for Python integration tests ..."
  virtualenv -p python3 ${VENV}
  if [ $? -ne 0 ]; then
    echo 'Error setting up the virtualenv'
    exit 1
  fi
fi
echo "Installing dependencies needed for Python integration tests ..."
${VENV}/bin/pip install -q -r ${TESTSFOLDER}/requirements.txt
if [ $? -ne 0 ]; then
  echo "Error installing required python packages in the virtualenv"
  exit 1
fi

# list installed depencencies and the versions of their dependencies
${VENV}/bin/python --version
${VENV}/bin/pip freeze

echo "Linting Python integration tests ..."
# We put the linting here for simplicity, since this is not a Python project
${VENV}/bin/flake8 --ignore E501,F401,F403,F405,W504,W605 ${TESTSFOLDER}/ --exclude='.venv*'
if [ $? -ne 0 ]; then
  echo 'Linting error'
  exit 1
fi

echo "Running Python integration tests ..."
${VENV}/bin/python -m unittest discover -s ${TESTSFOLDER}/ -p '*.py*' -v
if [ $? -ne 0 ]; then
  echo 'Test error'
  exit 1
fi

echo "Cleaning up object stores as the test is complete ..."
${VENV}/bin/python ${TESTSFOLDER}/clean_object_stores_after_tests.py
if [ $? -ne 0 ]; then
  echo 'Post test cleanup error'
  exit 1
fi
