package main

import (
	"flag"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/command"
)

const (
	packageName = "cri-o"
	oscCmd      = "osc"
)

var (
	targetVersionStr string
	dryRun           bool
	prefix           = "devel:kubic:libcontainers:stable:" + packageName
)

func main() {
	// Parse CLI flags.
	flag.StringVar(&targetVersionStr,
		"target-version", "", "The version to be upgraded to",
	)
	flag.BoolVar(&dryRun,
		"dry-run", true, "Just do a dry run",
	)
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	logrus.SetLevel(logrus.DebugLevel)
	command.SetGlobalVerbose(true)

	if err := run(); err != nil {
		logrus.Fatalf("%v", err)
	}
}

func run() error {
	if targetVersionStr == "" {
		return errors.New("--target-version must be specified")
	}

	pv, err := New(targetVersionStr)
	if err != nil {
		return errors.Wrapf(err, "parse targetVersionStr")
	}

	if err := pv.CreatePackage(); err != nil {
		return err
	}

	return nil
}
