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
	defaultOrgs = "bitrise-steplib,bitrise-io,bitrise-community"
)

var (
	mode   string
	repoSrc string
	orgs   string
)

func init() {
	flag.StringVar(&mode, "mode", defaultMode, "--mode=dry|live (dry: only prints what would happen, but modifies nothing)")
	flag.StringVar(&repoSrc, "repo-src", defaultRepoSrc, "--repo-src=cherry|steplib (repo loader to use to process arguments)")
	flag.StringVar(&orgs, "orgs", defaultOrgs, "--orgs=bitrise-steplib,bitrise-io (filters step repos to those owned by given orgs)")
}

func main() {

	flag.Parse()
	
	if len(flag.Args()) == 0 {
		log.Errorf("no repo source url specified")
		os.Exit(1)
	}

	var repoURLs []string
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

	issues := getOpenIssues(repoURLs)
	var stats runStats
	switch mode {
	case "dry":
		dryRun(issues)
	case "live":
		liveRun(issues)
	default:
		log.Errorf("unkown run mode %s", mode)
		os.Exit(1)
	}

	log.Successf("finished processing issues!")
	log.Printf("run stats:")
	log.Printf("open/pr/stale/migrated: %d/%d/%d/%d ", stats.Processed, stats.PullRequest, stats.Stale, stats.Active)
}
