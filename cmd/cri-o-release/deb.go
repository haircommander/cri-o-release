package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	kgit "k8s.io/release/pkg/git"
	"sigs.k8s.io/release-utils/command"
)

func (p *projectVersion) bumpDeb() error {
	// TODO FIXME helper
	p.oldProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor-1)
	p.newProject = fmt.Sprintf("%s:%d.%d", prefix, p.version.Major, p.version.Minor)

	debian, err := cloneOrOpenDebian()
	if err != nil {
		return err
	}
	if err := debian.Checkout(p.DebianBranchName()); err != nil {
		return err
	}

	upstream, err := cloneOrOpenUpstream()
	if err != nil {
		return err
	}

	if err := upstream.Checkout(p.Version()); err != nil {
		return err
	}

	if err := p.bumpDebianChangelog(); err != nil {
		return err
	}

	linesToReplace := map[string]string{
		"UPSTREAM_TAG": "UPSTREAM_TAG=" + p.Version(),
	}

	if err := replaceLinesInFile(fileInDebianRepo("rules"), linesToReplace); err != nil {
		return err
	}
	if !dryRun {
		if err := debian.Add(fileInDebianRepo("rules")); err != nil {
			return err
		}
		if err := debian.Add(fileInDebianRepo("changelog")); err != nil {
			return err
		}
		msg := "bump to " + p.Version()

		if err := debian.UserCommit(msg); err != nil {
			return err
		}
	}

	if err := os.Rename(fileInDebianRepo(""), filepath.Join(upstreamRepoPath, "debian")); err != nil {
		return err
	}
	defer func() {
		if err := os.Rename(filepath.Join(upstreamRepoPath, "debian"), fileInDebianRepo("")); err != nil {
			logrus.Infof("failed to return debian path: %v", err)
		}
	}()

	// TODO FIXME commit debian

	if err := os.Chdir(upstreamRepoPath); err != nil {
		return err
	}
	if err = command.New(
		"dpkg-buildpackage", "-us", "-uc", "-d",
	).RunSilentSuccess(); err != nil {
		return err
	}

	if err := os.Chdir(workdir); err != nil {
		return err
	}

	if err := p.copyRelevantDeb(upstreamRepoParent, p.obsPackageDir()); err != nil {
		return err
	}

	if err := os.Chdir(p.obsPackageDir()); err != nil {
		return err
	}
	if err := commitAllInWd(); err != nil {
		return err
	}
	return nil
}

func cloneOrOpenDebian() (*kgit.Repo, error) {
	// Checkout the release branch
	return kgit.CloneOrOpenRepo(debianRepoPath, "https://gitlab.com/rhcontainerbot/cri-o.git", false)
}

func (p *projectVersion) DebianBranchName() string {
	return fmt.Sprintf("debian-%d.%d", p.version.Major, p.version.Minor)
}

const debianChangelogPrepend = `cri-o (%s~0) stable; urgency=medium

  * bump to %s

`

func (p *projectVersion) bumpDebianChangelog() error {
	tmpCl, err := ioutil.TempFile(workdir, "changelog")
	if err != nil {
		return err
	}
	defer tmpCl.Close()

	if _, err := tmpCl.Write([]byte(fmt.Sprintf(debianChangelogPrepend, p.version.String(), p.Version()))); err != nil {
		return err
	}

	oldCl, err := os.Open(fileInDebianRepo("changelog"))
	if err != nil {
		return err
	}

	oldContents, err := ioutil.ReadAll(oldCl)
	if err != nil {
		return err
	}

	if _, err := tmpCl.Write(oldContents); err != nil {
		return err
	}

	tmpCl.Sync()

	return os.Rename(tmpCl.Name(), oldCl.Name())
}

func fileInDebianRepo(file string) string {
	return filepath.Join(debianRepoPath, "debian", file)
}
