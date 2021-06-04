package main

import (
	"fmt"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/command"
)

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

func (p *projectVersion) Validate(projects []string) error {
	for _, project := range projects {
		if project == p.oldProject {
			return nil
		}
	}
	return errors.Errorf("project %s not found", p.oldProject)
}

func (p *projectVersion) CreateProject() error {
	// TODO make pipe dry run
	cmd := command.New(oscCmd, "meta", "prj", p.oldProject).Pipe(
		"sed", "--expression", "s/project name=.*>/project name=\""+p.newProject+"\">/g",
	).Pipe(oscCmd, "meta", "prj", p.newProject, "-F", "-")

	output, err := cmd.RunSilentSuccessOutput()
	logrus.Infof("%s %v", output.OutputTrimNL(), err)
	return nil
}
