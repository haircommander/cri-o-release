package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/blang/semver"
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
	logrus.Infof("%+v", pv)

	projects, err := findCrioProjects()
	if err != nil {
		return err
	}
	logrus.Infof("%+v", projects)

	if err := pv.validateProjectVersions(projects); err != nil {
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

type projectVersion struct {
	version    *semver.Version
	oldProject string
	newProject string
}

func New(versionString string) (*projectVersion, error) {
	v, err := semver.Make(versionString)
	if err != nil {
		return nil, err
	}
	if v.Minor == 0 {
		return nil, errors.New("0 minor version not supported")
	}
	if v.Major == 0 {
		return nil, errors.New("0 major version not supported")
	}

	pv := &projectVersion{
		version: &v,
	}
	pv.setProjectVersions()
	return pv, nil
}

func (p *projectVersion) setProjectVersions() {
	if p.minorUpgrade() {
		p.oldProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor-1)
		p.newProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor)
		return
	}
	p.oldProject = fmt.Sprintf("%s:%d.%d:%d.%d.%d", prefix, p.version.Major, p.version.Minor, p.version.Major, p.version.Minor, p.version.Patch-1)
	p.newProject = fmt.Sprintf("%s:%d.%d:%d.%d.%d", prefix, p.version.Major, p.version.Minor, p.version.Major, p.version.Minor, p.version.Patch)
	return
}

func (p *projectVersion) minorUpgrade() bool {
	return p.version.Patch == 0
}

func (p *projectVersion) validateProjectVersions(projects []string) error {
	for _, project := range projects {
		if project == p.oldProject {
			return nil
		}
	}
	return errors.Errorf("project %s not found", p.oldProject)
}
