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
	registrationPIN := flag.String("registration-pin", "123456", "pairing emulator PIN")
	flag.Parse()

	sim := dptest.Start(dptest.NewState(*model, *firmware))
	defer sim.Close()
	if err := sim.SetRegistrationPIN(*registrationPIN); err != nil {
		fmt.Fprintln(os.Stderr, "dp-sim:", err)
		os.Exit(2)
	}
	fmt.Printf("dp-sim listening at %s\n", sim.URL())
	fmt.Printf("registration listening at %s\n", sim.RegistrationURL())
	fmt.Printf("certificate sha256 %s\n", sim.CertificateSHA256())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
