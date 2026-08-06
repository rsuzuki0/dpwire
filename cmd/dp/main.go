package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.0.0-p0"

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: dp version")
		fmt.Fprintln(flag.CommandLine.Output(), "\nP0 exposes no device operations.")
	}
	flag.Parse()
	if flag.NArg() == 1 && flag.Arg(0) == "version" {
		fmt.Println(version)
		return
	}
	flag.Usage()
	if flag.NArg() != 0 {
		os.Exit(2)
	}
}
