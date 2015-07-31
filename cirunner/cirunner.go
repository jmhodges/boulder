// Run all tests and coverage checks. Called from Travis automatically, also
// suitable to run manually. See list of prerequisite packages in .travis.yml
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var testDirs = []string{
	"analysis",
	"ca",
	"core",
	"log",
	"policy",
	"ra",
	"rpc",
	"sa",
	"test",
	"va",
	"wfe",
}

var (
	travisEncryptKeyName = "encrypted_53b2630f0fb4_key"
	travisEncryptIVName  = "encrypted_53b2630f0fb4_iv"
)

func main() {
	isTravisRun := os.Getenv("TRAVIS") == "true"
	triggerCommit := os.Getenv("TRAVIS_COMMIT")
	isPullRequest := os.Getenv("TRAVIS_PULL_REQUEST") != "false"
	travisEncryptKey := os.Getenv(travisEncryptKeyName)
	travisEncryptIV := os.Getenv(travisEncryptIVName)
	// If a non-maintainer PR or branch is running this code (that is,
	// it was a PR or branch from a fork of letsencypt/boulder), then
	// we are not given the encryption key and IV from TravisCI and so
	// need to not use it. Currently, the only use of them is to
	// decrypt the GitHub auth key we keep to make a nicer PR
	// integration on CI runs in TravisCI.
	isMaintainersPrOrBranch := travisEncryptIV != ""
	statusUpdater := nullUpdater{}
	if isTravisRun && isMaintainersPrOrBranch {
		statusUpdater = &githubUpdater{refSha: triggerCommit, buildId: os.Getenv("TRAVIS_BUILD_ID")}
	}
	if !isPullRequest {
		revs = outputCommand("git", "rev-list", "--parents", "-n", "1", "HEAD")
		rs := strings.Split(revs, " ")
		if len(rs) > 0 {
			triggerCommit = rs[len(rs)-1]
		}
	}
}

func sideEffectCommand(command string, args ...string) *exec.Cmd {
	c := exec.Command(command, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

func outputCommand(command string, args ...string) string {
	c := exec.Command(command, args...)
	stdout := new(bytes.Buffer)
	c.Stdout = stdout
	stderr := new(bytes.Buffer)
	c.Stderr = stderr
	err := c.Run()
	if err != nil {
		os.Stdout.Write(stdout.Bytes())
		os.Stderr.Write(stderr.Bytes())
		log.Fatalf("unable to run command `%s %s`", command, strings.Join(" ", args))
	}
	if err != nil {
		os.Stdout.Write(stdout.Bytes())
		os.Stderr.Write(stderr.Bytes())
		log.Fatalf("unable to get output of command `%s %s`", command, strings.Join(" ", args))
	}
	return string(c.Stdout.Bytes())
}

type prState int

var (
	success prState = iota
	failure
	error
)

var prStateToStr = map[prState]string{
	success: "success",
	failure: "failure",
	error:   "error",
}

type prUpdater interface {
	Update(st prState, description string) error
	Comment(result string) error
}

type githubUpdater struct {
	refSha  string
	buildID string
	prID    int
}

func (g *githubUpdater) Update(st prState, context string, description string) error {
	stateStr := prStateToStr[st]
	u := fmt.Sprintf("https://travis-ci.org/letsencrypt/boulder/builds/%s", g.buildID)
	args := []string{
		"--authfile", githubSecretFile,
		"--state", stateStr,
		"--owner", repoOwner,
		"--repo", repoName,
		"status",
		"--sha", g.refSha,
		"--url", u,
	}

	if context != "" {
		args = append(args, "--context", context)
	}
	if description != "" {
		args = append(args, "--description", description)
	}
	return sideEffectCommand("github-pr-status", args...).Run()
}

func (g *githubUpdater) Comment(body string) error {
	args := []string{
		"--authfile", githubSecretFile,
		"--owner", repoOwner,
		"--repo", repoName,
		"comment",
		"--pr", g.prID,
	}
	cmd := sideEffectCommand("github-pr-status", args...)
	cmd.Stdin = strings.NewReader("```\n" + body + "\n```")
	return cmd.Run()
}

type nullUpdater struct{}

func (n nullUpdater) Update(st prState, description string) error {
	return nil
}

func (n nullUpdater) Comment(result string) error {
	return nil
}
