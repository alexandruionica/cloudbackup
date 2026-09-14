# Run the Python integration tests on Windows. Same tier arguments as integration_tests.sh:
#   .\integration_tests.ps1            -> acceptance + api
#   .\integration_tests.ps1 cloud      -> cloud tier + object store cleanup
#   .\integration_tests.ps1 all        -> every tier
param([string[]]$Tiers = @())
try {

	$TESTSFOLDER='.\integration_tests'

	if(!(Test-Path -Path "$TESTSFOLDER\.venv_windows\Scripts\python.exe"  )){
	   python -m venv "$TESTSFOLDER\.venv_windows"
	   if ( $LastExitCode -ne 0 ) {
		exit $LastExitCode
	   } 
	}

	if(!(Test-Path -Path "$TESTSFOLDER\.venv_windows\Scripts\pip.exe"  )){
	  echo "Error: pip binary is missing. Can't proceed to install dependencies"
	  exit 5
	} 

	echo "Installing dependencies needed for Python integration tests ..."
	& "$TESTSFOLDER\.venv_windows\Scripts\pip.exe" install -q -r ${TESTSFOLDER}\requirements.txt
	if ( $LastExitCode -ne 0 ) {
	  echo "Error installing required python packages in the virtualenv"
	  exit $LastExitCode
	}

	& "$TESTSFOLDER\.venv_windows\Scripts\python.exe" --version
	& "$TESTSFOLDER\.venv_windows\Scripts\pip.exe" freeze
        if ( $LastExitCode -ne 0 ) {
          echo "Error listing installed python modules and their dependencies versions"
          exit $LastExitCode
        }


	if(!(Test-Path -Path "$TESTSFOLDER\.venv_windows\Scripts\flake8.exe"  )){
	  echo "Error: flake8 binary is missing. Can't proceed to lint python code"
	  exit 5
	}

	echo "Linting Python integration tests ..."
	# We put the linting here for simplicity, since this is not a Python project
	& "$TESTSFOLDER\.venv_windows\Scripts\flake8.exe" --ignore E501,F401,F403,F405,W504,W605 ${TESTSFOLDER} --exclude=.venv_windows --extend-exclude=.venv_linux
	if ( $LastExitCode -ne 0 ) {
	  echo 'Linting error'
	  exit $LastExitCode
	}

	if ($Tiers.Count -eq 0) { $Tiers = @("acceptance", "api") }
	elseif ($Tiers -contains "all") { $Tiers = @("acceptance", "api", "cloud") }
	$Paths = @()
	foreach ($tier in $Tiers) {
	  if (!(Test-Path -Path "$TESTSFOLDER\$tier" -PathType Container)) {
	    echo "Unknown tier '$tier' (expected a directory under $TESTSFOLDER)"
	    exit 1
	  }
	  $Paths += "$TESTSFOLDER\$tier"
	}

	echo "Running Python integration tests (tiers: $Tiers) ..."
	& "$TESTSFOLDER\.venv_windows\Scripts\python.exe" -m pytest @Paths -v
	if ( $LastExitCode -ne 0 ) {
	  exit $LastExitCode
	}

	if ($Tiers -contains "cloud") {
	  echo "Cleaning up object stores as the cloud tier is complete ..."
	  & "$TESTSFOLDER\.venv_windows\Scripts\python.exe" "$TESTSFOLDER\cloud\clean_object_stores_after_tests.py"
	  if ( $LastExitCode -ne 0 ) {
		exit $LastExitCode
	  }
	}
}
catch {
	echo "Encountered an exception"
	echo $_.Exception|format-list -force
	exit 6
}
