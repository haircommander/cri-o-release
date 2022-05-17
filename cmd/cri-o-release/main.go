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
	doRPMBump        bool
	doDebBump        bool
	prefix           = "devel:kubic:libcontainers:stable:" + packageName
)

func main() {
	// Parse CLI flags.
	flag.StringVar(&targetVersionStr,
		"version", "", "The version to be upgraded to",
	)
	flag.BoolVar(&dryRun,
		"dry-run", false, "Just do a dry run",
	)
	flag.BoolVar(&doRPMBump,
		"rpm", false, "bump the version of the RPM",
	)
	flag.BoolVar(&doDebBump,
		"deb", false, "bump the version of the deb",
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
	if pv.minorUpgrade() {
		if err := pv.createPackage(); err != nil {
			return err
		}
		if err := pv.validate(); err != nil {
			return err
		}
	}

	if err := pv.populateOscDirectories(); err != nil {
		return err
	}

	if doRPMBump {
		if err := pv.bumpRPM(); err != nil {
			return err
		}
	}

	if doDebBump {
		if err := pv.bumpDeb(); err != nil {
			return err
		}
	}

	if err := pv.branchProject(); err != nil {
		return err
	}

	return nil
}
