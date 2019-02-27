package main

import (
	"fmt"
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

func getRepoURLs(repoSrc string, srcStr string) ([]string, error) {
	var repoURLs []string
	var err error
	switch repoSrc {
	case "steplib":
		fromOrgs := strings.Split(orgs, ",")
		repoURLs, err = steplib.LoadRepos(srcStr, fromOrgs)
		if err != nil {
			return nil, fmt.Errorf("load repos from steplib: %s")
		}

		return repoURLs, nil
	case "cherry":
		return strings.Split(srcStr, ","), nil
	default:
		return nil, fmt.Errorf("error: not recognized repo source %s", repoSrc)
	}
}

func main() {

	flag.Parse()
	
	if len(flag.Args()) == 0 {
		log.Errorf("error: no repo source url specified")
		os.Exit(1)
	}

	log.Infof("get repos")
	repoURLs, err := getRepoURLs(repoSrc, flag.Args()[0])
	if err != nil {
		log.Errorf("error getting repos using mode %s and arg %s: %s", repoSrc, flag.Args()[0], err)
		os.Exit(1)
	}
	log.Printf("loaded %d repos: %s", len(repoURLs), repoURLs)
	
	log.Infof("get open issues")
	issues := github.GetOpenIssues(repoURLs)
	log.Printf("found %d open issues: %s", len(issues), github.GetHTMLURLs(issues))

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
		log.Errorf("error: %s", err)
		os.Exit(1)
	}

	log.Successf("success!")
	log.Printf("run stats:")
	log.Printf("open/pr/stale/migrated: %d/%d/%d/%d ", stats.Processed, stats.PullRequest, stats.Stale, stats.Active)
}
