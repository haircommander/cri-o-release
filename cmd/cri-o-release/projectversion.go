package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/command"
)

const (
	workdir string = "/tmp/cri-o-release-workdir"
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
	projects, err := oscLs("/", prefix)
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

	pkgs, err := oscLs(p.oldProject, "")
	if err != nil {
		return err
	}

	for _, pkg := range pkgs {
		// don't branch the cri-o package, as that shouldn't be a branch
		// or else edits to the old version will edit the new
		if pkg == packageName {
			continue
		}
		output, err := command.New(
			oscCmd, "branch", p.oldProject, pkg, p.newProject,
		).RunSilentSuccessOutput()
		if err != nil {
			logrus.Errorf("failed to branch: %v", err)
		} else {
			logrus.Debugf("osc branch output: %s", output.OutputTrimNL())
		}
	}

	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}

	if err := os.Chdir(workdir); err != nil {
		return err
	}

	// Only create if the project wasn't created
	if _, staterr := os.Stat(filepath.Join(workdir, p.newProject)); staterr != nil {
		if err = command.New(
			oscCmd, "co", p.newProject, "-M",
		).RunSilentSuccess(); err != nil {
			return err
		}
	}

	if err := os.Chdir(p.newProject); err != nil {
		return err
	}

	// Only create if the package wasn't created
	if _, staterr := os.Stat(filepath.Join(workdir, p.newProject, packageName)); staterr != nil {
		if err = command.New(
			oscCmd, "mkpac", packageName,
		).RunSilentSuccess(); err != nil {
			return err
		}
	}

	return nil
}

func oscLs(target string, grepTarget string) ([]string, error) {
	cmd := command.New(oscCmd, "ls", target)
	if grepTarget != "" {
		cmd = cmd.Pipe("grep", grepTarget)
	}
	output, err := cmd.RunSilentSuccessOutput()
	if err != nil {
		return nil, err
	}
	return strings.Split(output.OutputTrimNL(), "\n"), nil
}
