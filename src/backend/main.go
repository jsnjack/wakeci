package main

import (
	"flag"
	"log"
	"net/http"
	"os"
)

// Logger is the main logger
var Logger *log.Logger

// PortFlag is a port on which the server should be started
var PortFlag *string

// Version is the version of the application calculated with monova
var Version string

func init() {
	PortFlag = flag.String("port", "8081", "Port to start the server on")
	flag.Parse()

	Logger = log.New(os.Stdout, "", log.Lmicroseconds|log.Lshortfile)
}

func main() {
	Logger.Printf("Listening on port %s...\n", *PortFlag)
	err := http.ListenAndServe(":"+*PortFlag, nil)
	if err != nil {
		Logger.Fatal(err)
	}
}
