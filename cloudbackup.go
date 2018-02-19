package main

//noinspection GoRedundantImportAlias
import (
	flags "github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
	"os"
	"cloudbackup/cliargs"
)
const loggingContext = "main"
var logger = log.WithFields(log.Fields{
	"context": loggingContext,
})

func main() {
	var args cliargs.Args
	// the logic of the program flow is actually in the "cloudbackup/misc" package the the below flags.Parse() is what
	//  starts it all
	_, err := flags.Parse(&args)
	if err != nil {
		os.Exit(1)
	}
	logger.Info("Eng of program")
}
