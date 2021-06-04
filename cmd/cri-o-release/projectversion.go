package main

import (
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/command"
)

const prjConf string = `
Release: <CI_CNT>.<B_CNT>%%{?dist}
%if "%_repository" == "CentOS_8" || "%_repository" == "CentOS_8_Stream"
ExpandFlags: module:go-toolset-rhel8
%endif
%if "%_repository" == "CentOS_8_Stream"
Prefer: centos-stream-release
%endif
`

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

	if err := pv.validate(); err != nil {
		return nil, err
	}

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

func (p *projectVersion) validate() error {
	// running ls on the root will list all packages
	projects, err := oscLs("/")
	if err != nil {
		return err
	}

	for _, project := range projects {
		if project == p.oldProject {
			return nil
		}
	}
	return errors.Errorf("project %s not found", p.oldProject)
}

func (p *projectVersion) CreateProject() error {
	// TODO make last pipe dry run
	// Update metadata for new project
	cmd := command.New(oscCmd, "meta", "prj", p.oldProject).Pipe(
		"sed", "--expression", "s/project name=.*>/project name=\""+p.newProject+"\">/g",
	).Pipe(oscCmd, "meta", "prj", p.newProject, "-F", "-")

	output, err := cmd.RunSilentSuccessOutput()
	if err != nil {
		return err
	}
	logrus.Debugf("osc meta prj output: %s", output.OutputTrimNL())

	// TODO also make this dry run
	// Update prjconf for new project
	cmd = command.New("echo", prjConf).Pipe(
		oscCmd, "meta", "prjconf", "-F", "-", p.newProject,
	)

	output, err = cmd.RunSilentSuccessOutput()
	if err != nil {
		return err
	}
	logrus.Debugf("osc meta prjconf output: %s", output.OutputTrimNL())

	return nil
}

func oscLs(target string) ([]string, error) {
	output, err := command.New(oscCmd, "ls", target).Pipe(
		"grep", prefix,
	).RunSilentSuccessOutput()
	if err != nil {
		return nil, err
	}
	return strings.Split(output.OutputTrimNL(), "\n"), nil
}
