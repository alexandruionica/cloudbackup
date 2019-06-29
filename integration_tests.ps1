try {

	$TESTSFOLDER='.\integration_tests'

	if(!(Test-Path -Path "$TESTSFOLDER\.venv\Scripts\python.exe"  )){
	   virtualenv "$TESTSFOLDER\.venv"
	   if ( $LastExitCode -ne 0 ) {
		exit $LastExitCode
	   } 
	}

	if(!(Test-Path -Path "$TESTSFOLDER\.venv\Scripts\pip.exe"  )){
	  echo "Error: pip binary is missing. Can't proceed to install dependencies"
	  exit 5
	} 

	echo "Installing dependencies needed for Python integration tests ..."
	& "$TESTSFOLDER\.venv\Scripts\pip.exe" install -q -r ${TESTSFOLDER}\requirements.txt
	if ( $LastExitCode -ne 0 ) {
	  echo "Error installing required python packages in the virtualenv"
	  exit $LastExitCode
	}

	& "$TESTSFOLDER\.venv\Scripts\python.exe" --version
	& "$TESTSFOLDER\.venv\Scripts\pip.exe" freeze
        if ( $LastExitCode -ne 0 ) {
          echo "Error listing installed python modules and their dependencies versions"
          exit $LastExitCode
        }


	if(!(Test-Path -Path "$TESTSFOLDER\.venv\Scripts\flake8.exe"  )){
	  echo "Error: flake8 binary is missing. Can't proceed to lint python code"
	  exit 5
	}

	echo "Linting Python integration tests ..."
	# We put the linting here for simplicity, since this is not a Python project
	& "$TESTSFOLDER\.venv\Scripts\flake8.exe" --ignore E501,F401,F403,F405 ${TESTSFOLDER} --exclude=.venv
	if ( $LastExitCode -ne 0 ) {
	  echo 'Linting error'
	  exit $LastExitCode
	}

	echo "Running Python integration tests ..."
	& "$TESTSFOLDER\.venv\Scripts\python.exe" -m unittest discover -s ${TESTSFOLDER} -p '*.py*' -v
	if ( $LastExitCode -ne 0 ) {
	  exit $LastExitCode
	}

	echo "Cleaning up object stores as the test is complete ..."
	& "$TESTSFOLDER\.venv\Scripts\python.exe" "$TESTSFOLDER\clean_object_stores_after_tests.py"
	if ( $LastExitCode -ne 0 ) {
		exit $LastExitCode
	}
}
catch {
	echo "Encountered an exception"
	echo $_.Exception|format-list -force
	exit 6
}
