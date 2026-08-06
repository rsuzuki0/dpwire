package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rsuzuki0/digitalpaper/dptest"
)

func main() {
	model := flag.String("model", "DP-SIM", "reported model name")
	firmware := flag.String("firmware", "0.0-p0", "reported firmware version")
	flag.Parse()

	sim := dptest.Start(dptest.NewState(*model, *firmware))
	defer sim.Close()
	fmt.Printf("dp-sim listening at %s\n", sim.URL())
	fmt.Printf("certificate sha256 %s\n", sim.CertificateSHA256())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
