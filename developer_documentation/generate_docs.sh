#!/bin/sh

export PYTHONIOENCODING='utf-8'
export DOCSSRCFOLDER='./'

if [ ! -x ${DOCSSRCFOLDER}/.venv/bin/python ]; then
  echo "Setting up virtualenv for Python docs generator ..."
  virtualenv -p python3 ${DOCSSRCFOLDER}/.venv
  if [ $? -ne 0 ]; then
    echo 'Error setting up the virtualenv'
    exit 1
  fi
fi
echo "Installing dependencies needed for Python docs generator ..."
${DOCSSRCFOLDER}/.venv/bin/pip install -q -r ${DOCSSRCFOLDER}/requirements.txt
if [ $? -ne 0 ]; then
  echo "Error installing required python packages in the virtualenv"
  exit 1
fi

# list installed depencencies and the versions of their dependencies
${DOCSSRCFOLDER}/.venv/bin/python --version
${DOCSSRCFOLDER}/.venv/bin/pip freeze

echo "Running Python documentation generator ..."
cd ${DOCSSRCFOLDER}/
.venv/bin/mkdocs build
