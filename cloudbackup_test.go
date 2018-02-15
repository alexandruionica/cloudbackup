package main

import (
	log "github.com/sirupsen/logrus"
	"testing"
	"cloudbackup/misc"
	"reflect"
)

// test that variables are of the expected type
func TestVars1(t *testing.T){
	var loggerHere = log.WithFields(log.Fields{
		"context": "main",
	})

	if reflect.TypeOf(logger) != reflect.TypeOf(loggerHere) {
		t.Fatal("Variable called 'logger' is not of expected type")
	}

}

// test that variables are of the expected type
func TestVars2(t *testing.T){
	var argsHere misc.Args

	if reflect.TypeOf(args) != reflect.TypeOf(argsHere) {
		t.Fatal("Variable called 'args' is not of expected type")
	}

}