package main

import (
	"flag"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/command"
)

const (
	packageName = "cri-o"
	prefix      = "devel:kubic:libcontainers:stable:" + packageName
	oscCmd      = "osc"
)

var (
	targetVersionStr string
	dryRun           bool
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

	projects, err := findCrioProjects()
	if err != nil {
		return err
	}

	if err := pv.Validate(projects); err != nil {
		return err
	}

	if err := pv.CreateProject(); err != nil {
		return err
	}

	return nil
}

func findCrioProjects() ([]string, error) {
	output, err := command.New(oscCmd, "ls", "/").Pipe(
		"grep", prefix,
	).RunSilentSuccessOutput()
	if err != nil {
		return nil, err
	}
	return strings.Split(output.OutputTrimNL(), "\n"), nil
}
