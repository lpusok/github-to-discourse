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
)

var (
	client *github.Client
	ctx    context.Context
	tc     *http.Client // todo: check if this can be eliminated
	mode   string
	loader string
	steplibFilter string
	orgs          []string
)

func init() {
	ctx = context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc = oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)

	flag.StringVar(&mode, "run-mode", defaultMode, "--runmode=dry|live (dry: only prints what would happen, but modifies nothing)")
	flag.StringVar(&loader, "repo-loader", "cherry", "--repo-loader=cherry|owner|steplib (repo loader to use to process arguments)")
	flag.StringVar(&steplibFilter, "steplib-filter", "bitrise-steplib,bitrise-io,bitrise-community", "--steplib-filter=bitrise-steplib,bitrise-io (filters step repos to those owned by given orgs)")
	orgs = strings.Split(steplibFilter, ",")
}

func main() {

	flag.Parse()

	if loader == "" && chkpt == "" {
		log.Errorf("error: must provide repo source or checkpoint file")
		os.Exit(1)
	}
	
	if len(flag.Args()) == 0 {
		log.Errorf("no argument specified")
		os.Exit(1)
	}

	var baseRepos []repo
	var err error

	switch loader {
	case "steplib":
		baseRepos, err = getFromStepLib(flag.Args(), steplibFilter)
	case "cherry":
		baseRepos, err = getFromList(flag.Args())
	default:
		log.Errorf("not recognized repo loader %s", loader)
		os.Exit(1)
	}

	log.Debugf("base repos loaded: %s", baseRepos)


	issues := getOpenIssues(baseRepos)

	log.Debugf("open github issues: %s", issues)

	log.Debugf("select run mode")
	runMode, err := getRunMode(mode)
	if err != nil {
		log.Errorf(fmt.Sprintf("error selecting run mode: %s", err))
		os.Exit(1)
	}

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
