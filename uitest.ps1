try {
	$nodeVer = $null
	$nodeMajor = 0
	try {
		$nodeVer = node --version 2>$null
		if ($LastExitCode -eq 0 -and $nodeVer) {
			$nodeMajor = [int](($nodeVer -replace '^v','') -split '\.')[0]
		}
	} catch {
		$nodeMajor = 0
	}
	if ($nodeMajor -lt 18) {
		$found = if ($nodeVer) { $nodeVer } else { "none" }
		echo "ERROR: web UI tests require Node.js >= 18 (found $found)."
		exit 1
	}

	Push-Location webstatic\ui

	echo "Installing Node.js dependencies for web UI tests ..."
	& npm install --no-audit --no-fund --silent
	if ($LastExitCode -ne 0) {
		echo "Error installing npm dependencies"
		exit $LastExitCode
	}

	echo "Running web UI tests ..."
	& npm test
	if ($LastExitCode -ne 0) {
		echo "Test error"
		exit $LastExitCode
	}

	Pop-Location
}
catch {
	echo "Encountered an exception"
	echo $_.Exception | Format-List -Force
	exit 6
}
