
$TESTSFOLDER='.\integration_tests'

if(!(Test-Path -Path "$TESTSFOLDER\.venv\Scripts\python.exe"  )){
   virtualenv "$TESTSFOLDER\.venv"
}


echo "Installing dependencies needed for Python integration tests ..."
& "$TESTSFOLDER\.venv\Scripts\pip.exe" install -q -r ${TESTSFOLDER}\requirements.txt
if ( $LastExitCode -ne 0 ) {
  echo "Error installing required python packages in the virtualenv"
  exit
  }

echo "Linting Python integration tests ..."
# We put the linting here for simplicity, since this is not a Python project
& "$TESTSFOLDER\.venv\Scripts\flake8.exe" --ignore E501,F401,F403,F405 ${TESTSFOLDER} --exclude=.venv
if ( $LastExitCode -ne 0 ) {
  echo 'Linting error'
  exit 
}

echo "Running Python integration tests ..."
& "$TESTSFOLDER\.venv\Scripts\python.exe" -m unittest discover -s ${TESTSFOLDER} -p '*.py*' -v

