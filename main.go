package main

import (
	"flag"
	"os"
	"strings"

	"github.com/bitrise-io/go-utils/log"
	"github.com/lszucs/github-to-discourse/internal/github"
	"github.com/lszucs/github-to-discourse/internal/steplib"
	"github.com/lszucs/github-to-discourse/internal/runmode"
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
		log.Errorf("error: no repo source url specified")
		os.Exit(1)
	}

	var repoURLs []string
	var err error
	switch repoSrc {
	case "steplib":
		fromOrgs := strings.Split(orgs, ",")
		steplibURL := flag.Args()[0]
		
		log.Printf("loading repos")
		repoURLs, err = steplib.LoadRepos(steplibURL, fromOrgs)
	case "cherry":
		repoURLs = strings.Split(flag.Args()[0], ",")
	default:
		log.Errorf("error: not recognized repo source %s", repoSrc)
		os.Exit(1)
	}

	if err != nil {
		log.Errorf("error: %s", mode)
		os.Exit(1)
	}
	
	log.Printf("get open issues for repos: %s", repoURLs)
	issues := github.GetOpenIssues(repoURLs)

	log.Printf("running in %s mode", mode)
	var stats runmode.Stats
	switch mode {
	case "dry":
		stats, err = runmode.DryRun(issues)
	case "live":
		stats, err = runmode.LiveRun(issues)
	default:
		log.Errorf("error: unkown run mode %s", mode)
		os.Exit(1)
	}
	
	if err != nil {
		log.Errorf("error: %s", mode)
		os.Exit(1)
	}

	log.Successf("finished processing issues!")
	log.Printf("run stats:")
	log.Printf("open/pr/stale/migrated: %d/%d/%d/%d ", stats.Processed, stats.PullRequest, stats.Stale, stats.Active)
}
