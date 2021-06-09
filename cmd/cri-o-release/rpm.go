package main

import (
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
	specFile string = packageName + ".spec"
)

func (p *projectVersion) bumpRPM() error {
	if err := os.Chdir(rpmSourceDir); err != nil {
		return err
	}
	// Checkout the correct branch
	repo, err := kgit.OpenRepo(".")
	if err != nil {
		return errors.Wrap(err, "unable to open this repository")
	}

	if err := repo.Checkout(p.RPMBranchName()); err != nil {
		return errors.Wrapf(err, "unable to checkout version %s", p.RPMBranchName())
	}

	rev, err := p.findReleaseGitCommit()
	if err != nil {
		return err
	}

	linesToReplace := map[string]string{
		"%define built_tag ": "%define built_tag " + p.Version(),
		"Version: ":          "Version: " + p.Version(),
		"%global commit0 ":   "%global commit0 " + rev,
	}

	if err := replaceLinesInFile("cri-o.spec", linesToReplace); err != nil {
		return err
	}

	if err = command.New(
		spectoolCmd, "-g", specFile,
	).RunSilentSuccess(); err != nil {
		return err
	}

	if err = command.New(
		bumpspecCmd, "-c", "bump to "+p.Version(), specFile,
	).RunSilentSuccess(); err != nil {
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
	repo, err := kgit.CloneOrOpenGitHubRepo(upstreamRepoPath, packageName, packageName, false)
	if err != nil {
		return "", errors.Wrap(err, "unable to open this repository")
	}
	rev, err := repo.RevParse(p.Version())
	if err != nil {
		return "", errors.Wrap(err, "unable to list tags")
	}

	return rev, nil
}

func (p *projectVersion) Version() string {
	return "v" + p.version.String()
}

func (p *projectVersion) RPMBranchName() string {
	return strconv.FormatUint(p.version.Major, 10) + "." + strconv.FormatUint(p.version.Minor, 10)
}
