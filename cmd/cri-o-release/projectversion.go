package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/blang/semver"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/command"
)

const (
	// TODO FIXME hardcoded
	workdir string = "/home/pehunt/Packaging/obs/cri-o-release-workdir"
	// TODO FIXME hardcoded
	rpmSourceDir string = "/home/pehunt/Packaging/fedora/cri-o"
)

var (
	upstreamRepoParent string = filepath.Join(workdir, "cri-o-upstream")
	upstreamRepoPath   string = filepath.Join(upstreamRepoParent, "cri-o")
	debianRepoPath     string = filepath.Join(workdir, "debian", "cri-o")
)

const prjConf string = `
Release: <CI_CNT>.<B_CNT>%%{?dist}
%if "%_repository" == "CentOS_8" || "%_repository" == "CentOS_8_Stream"
ExpandFlags: module:go-toolset-rhel8
%endif
%if "%_repository" == "CentOS_8_Stream"
Prefer: centos-stream-release
%endif
Prefer: golang-github-cpuguy83-go-md2man
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

	return pv, nil
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

func (p *projectVersion) createPackage() error {
	p.oldProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor-1)
	p.newProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor)

	return p.createProject()
}

func (p *projectVersion) branchProject() error {
	p.oldProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor)
	p.newProject = fmt.Sprintf("%s:%d.%d:%d.%d.%d", prefix, p.version.Major, p.version.Minor, p.version.Major, p.version.Minor, p.version.Patch)

	if err := p.createProject(); err != nil {
		return err
	}

	if err := p.copyPackage(); err != nil {
		return err
	}

	return nil
}

func (p *projectVersion) copyPackage() error {
	if err := enterWorkdir(); err != nil {
		return err
	}
	if err := oscCo(p.oldProject, false); err != nil {
		return err
	}
	if err := oscCo(p.newProject, false); err != nil {
		return err
	}

	// make sure we're up to date
	if err := os.Chdir(filepath.Join(p.newProject, packageName)); err != nil {
		return err
	}
	command.New(oscCmd, "update").RunSilentSuccess()
	if err := os.Chdir(workdir); err != nil {
		return err
	}

	logrus.Debugf("copying %s to %s", p.oldProject, p.newProject)
	if err := p.copyRelevant(filepath.Join(p.oldProject, packageName), filepath.Join(p.newProject, packageName)); err != nil {
		return err
	}
	if err := os.Chdir(filepath.Join(p.newProject, packageName)); err != nil {
		return err
	}
	if err := commitAllInWd(); err != nil {
		return err
	}

	return nil
}

func commitAllInWd() error {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return err
	}
	fileNames := make([]string, 0, len(files))
	for _, file := range files {
		if file.Name() == ".osc" {
			continue
		}
		if file.Name() == "_meta" {
			continue
		}
		fileNames = append(fileNames, file.Name())
	}

	if err := command.New(oscCmd, append([]string{"add"}, fileNames...)...).RunSilentSuccess(); err != nil {
		return err
	}
	if err := command.New(oscCmd, "commit", "-n").RunSilentSuccess(); err != nil {
		return err
	}
	return nil
}

func (p *projectVersion) createProject() error {
	if err := p.copyProjectMeta(); err != nil {
		return err
	}
	if err := p.createPrjConf(); err != nil {
		return err
	}

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

	if err := p.enterNewProject(); err != nil {
		return err
	}

	// Only create if the package wasn't created
	if _, staterr := os.Stat(p.obsPackageDir()); staterr != nil {
		if err = command.New(
			oscCmd, "mkpac", packageName,
		).RunSilentSuccess(); err != nil {
			return err
		}
	}

	return nil
}

func (p *projectVersion) enterNewProject() error {
	if err := enterWorkdir(); err != nil {
		return err
	}

	if err := oscCo(p.newProject, true); err != nil {
		return err
	}

	return os.Chdir(p.newProject)
}

func oscCo(project string, meta bool) error {
	if _, err := os.Stat(project); err == nil {
		return nil
	}
	args := []string{"co", project}
	if meta {
		args = append(args, "-M")
	}
	cmd := command.New(oscCmd, args...)
	output, err := cmd.RunSilentSuccessOutput()
	if err != nil {
		return err
	}
	logrus.Debugf("osc co", output.OutputTrimNL())
	return nil
}

func (p *projectVersion) copyProjectMeta() error {
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
	return nil
}

func (p *projectVersion) createPrjConf() error {
	// TODO also make this dry run
	// Update prjconf for new project
	cmd := command.New("echo", prjConf).Pipe(
		oscCmd, "meta", "prjconf", "-F", "-", p.newProject,
	)

	output, err := cmd.RunSilentSuccessOutput()
	if err != nil {
		return err
	}
	logrus.Debugf("osc meta prjconf output: %s", output.OutputTrimNL())
	return nil
}

func (p *projectVersion) obsPackageDir() string {
	return filepath.Join(p.obsProjectDir(), packageName)
}

func (p *projectVersion) obsProjectDir() string {
	return filepath.Join(workdir, p.newProject)
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

func (p *projectVersion) copyRelevant(src, dest string) error {
	if err := p.copyRelevantRPM(src, dest); err != nil {
		return err
	}
	return p.copyRelevantDeb(src, dest)
}

func (p *projectVersion) copyRelevantRPM(src, dest string) error {
	return copy.Copy(src, dest, copy.Options{
		Skip: func(src string) (bool, error) {
			logrus.Debugf("checking %s", src)
			skip := !strings.HasSuffix(src, "sysconfig") &&
				!strings.HasSuffix(src, "cri-o.spec") &&
				!strings.HasSuffix(src, p.RPMTarGz()) &&
				!strings.HasSuffix(src, p.LegacyRPMTarGz())
			if skip {
				logrus.Debugf("skipping")
			}
			return skip, nil
		},
	})
}

func (p *projectVersion) copyRelevantDeb(src, dest string) error {
	return copy.Copy(src, dest, copy.Options{
		Skip: func(src string) (bool, error) {
			logrus.Debugf("checking %s", src)
			skip := !strings.HasSuffix(src, fmt.Sprintf("cri-o_%s~0.dsc", p.version.String())) &&
				!strings.HasSuffix(src, fmt.Sprintf("cri-o_%s~0.tar.gz", p.version.String()))
			if skip {
				logrus.Debugf("skipping")
			}
			return skip, nil
		},
	})
}

func (p *projectVersion) populateOscDirectories() error {
	if err := enterWorkdir(); err != nil {
		return err
	}
	if p.minorUpgrade() {
		p.oldProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor-1)
		p.newProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor)
		if err := oscCo(p.oldProject, false); err != nil {
			return err
		}
		if err := oscCo(p.newProject, false); err != nil {
			return err
		}
	}

	p.oldProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor)
	if err := oscCo(p.oldProject, false); err != nil {
		return err
	}
	return nil
}

func enterWorkdir() error {
	if err := os.Chdir(workdir); err != nil {
		if !errors.Is(err, syscall.ENOENT) {
			return err
		}
		if err := os.MkdirAll(workdir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
