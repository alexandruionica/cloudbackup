package globals

import "testing"

func TestIncrementAndDecrementFunctions1(t *testing.T) {
	funcName := "MyFunkyFunction"
	Stats.IncrementFunctions(funcName)
	if Stats.Gauge.Functions[funcName] != 1 {
		t.Fatalf("Was expecting the gauge for function '%s' to have value 1 but instead it is %d", funcName,
			Stats.Gauge.Functions[funcName])
	}

	if Stats.Counter.Functions[funcName] != 1 {
		t.Fatalf("Was expecting the counter for function '%s' to have value 1 but instead it is %d", funcName,
			Stats.Counter.Functions[funcName])
	}

	Stats.DecrementFunctions(funcName)
	if Stats.Gauge.Functions[funcName] != 0 {
		t.Fatalf("Was expecting the gauge for function '%s' to have value 0 but instead it is %d", funcName,
			Stats.Gauge.Functions[funcName])
	}

	if Stats.Counter.Functions[funcName] != 1 {
		t.Fatalf("Was expecting the counter for function '%s' to have value 1 but instead it is %d", funcName,
			Stats.Counter.Functions[funcName])
	}

	// attempt to decrement below 0
	Stats.DecrementFunctions(funcName)
	if Stats.Gauge.Functions[funcName] != 0 {
		t.Fatalf("Was expecting the gauge for function '%s' to have value 0 but instead it is %d", funcName,
			Stats.Gauge.Functions[funcName])
	}

	if Stats.Counter.Functions[funcName] != 1 {
		t.Fatalf("Was expecting the counter for function '%s' to have value 1 but instead it is %d", funcName,
			Stats.Counter.Functions[funcName])
	}
}

// attempt to decrement unintialized function name
func TestIncrementAndDecrementFunctions2(t *testing.T) {
	funcName := "MyFunkyFunction2"

	// attempt to decrement unintialized function name
	Stats.DecrementFunctions(funcName)
	// If the requested key doesn't exist, we get the value type's zero value
	if Stats.Gauge.Functions[funcName] != 0 {
		t.Fatalf("Was expecting the gauge for function '%s' to have value 0 but instead it is %d", funcName,
			Stats.Gauge.Functions[funcName])
	}
	// If the requested key doesn't exist, we get the value type's zero value
	if Stats.Counter.Functions[funcName] != 0 {
		t.Fatalf("Was expecting the counter for function '%s' to have value 0 but instead it is %d", funcName,
			Stats.Counter.Functions[funcName])
	}
}

func TestIncrementAndDecrementRoutines1(t *testing.T) {
	routineName := "MyFunkyRoutine"
	Stats.IncrementRoutines(routineName)
	if Stats.Gauge.Routines[routineName] != 1 {
		t.Fatalf("Was expecting the gauge for routine '%s' to have value 1 but instead it is %d", routineName,
			Stats.Gauge.Routines[routineName])
	}

	if Stats.Counter.Routines[routineName] != 1 {
		t.Fatalf("Was expecting the counter for routineName '%s' to have value 1 but instead it is %d",
			routineName, Stats.Counter.Routines[routineName])
	}

	Stats.DecrementRoutines(routineName)
	if Stats.Gauge.Routines[routineName] != 0 {
		t.Fatalf("Was expecting the gauge for routine '%s' to have value 0 but instead it is %d", routineName,
			Stats.Gauge.Routines[routineName])
	}

	if Stats.Counter.Routines[routineName] != 1 {
		t.Fatalf("Was expecting the counter for routineName '%s' to have value 1 but instead it is %d",
			routineName, Stats.Counter.Routines[routineName])
	}

	// attempt to decrement below 0
	Stats.DecrementRoutines(routineName)
	if Stats.Gauge.Routines[routineName] != 0 {
		t.Fatalf("Was expecting the gauge for routine '%s' to have value 0 but instead it is %d", routineName,
			Stats.Gauge.Routines[routineName])
	}

	if Stats.Counter.Routines[routineName] != 1 {
		t.Fatalf("Was expecting the counter for routineName '%s' to have value 1 but instead it is %d",
			routineName, Stats.Counter.Routines[routineName])
	}
}

// attempt to decrement routine function name
func TestIncrementAndDecrementRoutines2(t *testing.T) {
	routineName := "MyFunkyRoutine2"
	// attempt to decrement routine function name
	Stats.DecrementRoutines(routineName)

	// If the requested key doesn't exist, we get the value type's zero value
	if Stats.Gauge.Routines[routineName] != 0 {
		t.Fatalf("Was expecting the gauge for routine '%s' to have value 0 but instead it is %d", routineName,
			Stats.Gauge.Routines[routineName])
	}

	// If the requested key doesn't exist, we get the value type's zero value
	if Stats.Counter.Routines[routineName] != 0 {
		t.Fatalf("Was expecting the counter for routineName '%s' to have value 0 but instead it is %d",
			routineName, Stats.Counter.Routines[routineName])
	}
}

// test that a call to Print and also to Log at least works
func TestPrintLog(t *testing.T) {
	Stats.Print()
	Stats.Log()
}