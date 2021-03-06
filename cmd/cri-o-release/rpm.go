package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	kgit "k8s.io/release/pkg/git"
	"sigs.k8s.io/release-utils/command"
)

const (
	spectoolCmd string = "spectool"
	bumpspecCmd string = "rpmdev-bumpspec"
)

var (
	specFile       string   = packageName + ".spec"
	sysconfigFiles []string = []string{"crio-network.sysconfig", "crio-storage.sysconfig", "crio-metrics.sysconfig"}
)

func (p *projectVersion) bumpRPM() error {
	// TODO FIXME helper
	p.oldProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor-1)
	p.newProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor)

	if err := os.Chdir(rpmSourceDir); err != nil {
		return err
	}
	// Checkout the correct branch
	repo, err := kgit.OpenRepo(".")
	if err != nil {
		return errors.Wrap(err, "unable to open this repository")
	}

	// Manual step: if minor version upgrade, rpm branch doesn't exist yet
	if err := repo.Checkout(p.RPMBranchName()); err != nil {
		return errors.Wrapf(err, "unable to checkout version %s", p.RPMBranchName())
	}

	rev, err := p.findReleaseGitCommit()
	if err != nil {
		return err
	}

	linesToReplace := map[string]string{
		"Version: ":        "Version:                " + p.version.String(),
		"Release: ":        "Release:        0%{?dist}",
		"%global commit0 ": "%global commit0 " + rev,
	}

	// Manual step: if minor version upgrade, cri-o.spec doesn't exist
	// can be found with `git checkout origin/$oldversion cri-o.spec
	if err := replaceLinesInFile("cri-o.spec", linesToReplace); err != nil {
		return err
	}

	linesToReplaceLegacy := map[string]string{
		"%define built_tag ": "%define built_tag " + p.Version(),
	}
	if err := replaceLinesInFile("cri-o.spec", linesToReplaceLegacy); err != nil {
		logrus.Debugf("failed to replace legacy line: %v", err)
	}

	if err = command.New(
		spectoolCmd, "-g", specFile,
	).RunSilentSuccess(); err != nil {
		return err
	}

	msg := "bump to " + p.Version()

	if err = command.New(
		bumpspecCmd, "-c", msg, specFile,
	).RunSilentSuccess(); err != nil {
		return err
	}

	if err := p.copyRelevantRPM(".", p.obsPackageDir()); err != nil {
		return err
	}

	if dryRun {
		return nil
	}
	if err := repo.Add(specFile); err != nil {
		return err
	}

	if err := command.New("fedpkg", "new-sources", p.RPMTarGz()).RunSilentSuccess(); err != nil {
		if err2 := command.New("fedpkg", "new-sources", p.LegacyRPMTarGz()).RunSilentSuccess(); err2 != nil {
			return err2
		}
	}
	if err := repo.UserCommit(msg); err != nil {
		return err
	}
	if err := os.Chdir(p.obsPackageDir()); err != nil {
		return err
	}
	if err := command.New(oscCmd, "update").RunSilentSuccess(); err != nil {
		logrus.Errorf("failed to update, package may not exist yet %v", err)
	}
	if err := commitAllInWd(); err != nil {
		return err
	}

	return nil

}

func replaceLinesInFile(file string, linesToReplace map[string]string) error {
	input, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	lines := strings.Split(string(input), "\n")

	for i, line := range lines {
		for searchLine, replaceLine := range linesToReplace {
			if strings.Contains(line, searchLine) {
				logrus.Infof("replacing %s with %s in %s", line, replaceLine, file)
				lines[i] = replaceLine
				delete(linesToReplace, searchLine)
				break
			}
		}
	}

	if len(linesToReplace) > 0 {
		return errors.Errorf("didn't replace all lines: missed %+v", linesToReplace)
	}

	output := strings.Join(lines, "\n")
	return ioutil.WriteFile(file, []byte(output), 0644)
}

func (p *projectVersion) findReleaseGitCommit() (string, error) {
	// Checkout the release branch
	repo, err := cloneOrOpenUpstream()
	if err != nil {
		return "", errors.Wrap(err, "unable to open this repository")
	}
	defer repo.Checkout("main")
	rev, err := repo.RevParse(p.Version())
	if err != nil {
		return "", errors.Wrap(err, "unable to list tags")
	}

	return rev, nil
}

func cloneOrOpenUpstream() (*kgit.Repo, error) {
	// Checkout the release branch
	return kgit.CloneOrOpenGitHubRepo(upstreamRepoPath, packageName, packageName, false)
}

func (p *projectVersion) Version() string {
	return "v" + p.version.String()
}

func (p *projectVersion) RPMBranchName() string {
	return strconv.FormatUint(p.version.Major, 10) + "." + strconv.FormatUint(p.version.Minor, 10)
}

func (p *projectVersion) RPMTarGz() string {
	return packageName + "-" + p.version.String() + ".tar.gz"
}

func (p *projectVersion) LegacyRPMTarGz() string {
	return p.Version() + ".tar.gz"
}
