package main

import (
	log "github.com/sirupsen/logrus"
	"reflect"
	"testing"
)

// test that variables are of the expected type
func TestVars1(t *testing.T) {
	var loggerHere = log.WithFields(log.Fields{
		"context": "main",
	})

	if reflect.TypeOf(logger) != reflect.TypeOf(loggerHere) {
		t.Fatal("Variable called 'logger' is not of expected type")
	}

}
