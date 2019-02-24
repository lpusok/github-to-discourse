package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/google/go-github/github"
	"github.com/bitrise-io/go-utils/log"
	"golang.org/x/oauth2"
)

const (
	defaultMode = "dry"
	defaultRepoSrc = "cherry"
)

var (
	client *github.Client
	ctx    context.Context
	tc     *http.Client // todo: check if this can be eliminated
	mode   string
	repoSrc string
	orgs   string
)

func init() {
	ctx = context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc = oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)

	flag.StringVar(&mode, "mode", defaultMode, "--mode=dry|live (dry: only prints what would happen, but modifies nothing)")
	flag.StringVar(&repoSrc, "repo-src", defaultRepoSrc, "--repo-src=cherry|steplib (repo loader to use to process arguments)")
	flag.StringVar(&orgs, "orgs", "bitrise-steplib,bitrise-io,bitrise-community", "--orgs=bitrise-steplib,bitrise-io (filters step repos to those owned by given orgs)")
}

func main() {

	flag.Parse()
	
	if len(flag.Args()) == 0 {
		log.Errorf("no repo source url specified")
		os.Exit(1)
	}

	var repoURLs []repoURL
	var err error

	switch repoSrc {
	case "steplib":
		repoURLs, err = getFromStepLib(flag.Args(), steplibFilter)
	case "cherry":
		repoURLs, err = flag.Args()
	default:
		log.Errorf("not recognized repo loader %s", loader)
		os.Exit(1)
	}

	log.Debugf("base repos loaded: %s", repoURLs)
	issues := getOpenIssues(repoURLs)

	log.Debugf("open github issues: %s", issues)

	log.Infof("start processing")
	switch mode {
	case "dry":
		dryRun()
	case "live":
		liveRun(tc)
	default:
		log.Errorf("unkown run mode %s", mode)
		os.Exit(1)
	}

	log.Successf("finished processing issues!")
	log.Printf("run stats:")
	log.Printf("open/pr/stale/migrated: %d/%d/%d/%d ", stats.Processed, stats.PullRequest, stats.Stale, stats.Active)
}
