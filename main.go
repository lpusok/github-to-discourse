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
	chkptLog    = "chkpt.log"
	defaultMode = "dry"
)

var (
	client *github.Client
	ctx    context.Context
	tc     *http.Client // todo: check if this can be eliminated
	mode   string
	loader string
	chkpt  string
	debug  string
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
	flag.StringVar(&chkpt, "chkpt", "", "--chkpt=checkpoint.log (continue from state stored in checkpoint file)")
	flag.StringVar(&debug, "debug", "", "--debug=true (if whatever value is present, debug mode is enabled)")
	flag.StringVar(&steplibFilter, "steplib-filter", "bitrise-steplib,bitrise-io,bitrise-community", "--steplib-filter=bitrise-steplib,bitrise-io (filters step repos to those owned by given orgs)")
	orgs = strings.Split(steplibFilter, ",")
}

func main() {

	flag.Parse()

	if loader == "" && chkpt == "" {
		log.Errorf("error: must provide repo source or checkpoint file")
		os.Exit(1)
	}

	if debug != "" {
		log.SetEnableDebugLog(true)
	}



	var baseRepos []repo
	var err error

	switch loader {
	case "steplib":
		baseRepos, err = getFromStepLib("https://bitrise-steplib-collection.s3.amazonaws.com/spec.json")
	case "cherry":
		baseRepos, err = getFromList(flag.Args(), steplibFilter)
	default:
		log.Errorf("not recognized repo loader %s", loader)
		os.Exit(1)
	}

	log.Debugf("base repos loaded: %s", baseRepos)

	if _, err := os.Stat(chkptLog); os.IsNotExist(err) {
		log.Infof("no checkpoint file -- creating now")
		if _, err := os.Create(chkptLog); err != nil {
			log.Errorf("error creating checkpoint file: %s", err)
			os.Exit(1)
		}
	}

	log.Debugf("load unfinished issue from checkpoint file")
	unfinished, state, err := unfinishedIssueLoader{}.Load()
	if err != nil {
		log.Errorf(fmt.Sprintf("error restoring unfinished issue: %s", err))
		os.Exit(1)
	}

	issues := openGithubIssueLoader{}.Load(baseRepos)

	log.Debugf("open github issues: %s", issues)

	log.Debugf("select run mode")
	runMode, err := getRunMode(mode)
	if err != nil {
		log.Errorf(fmt.Sprintf("error selecting run mode: %s", err))
		os.Exit(1)
	}

	log.Infof("start processing")
	stats, err := runMode.run(issues, unfinished, state)
	if err != nil {
		log.Errorf(fmt.Sprintf("error running in %s mode: %s", mode, err))
		os.Exit(1)
	}

	log.Successf("finished processing issues!")
	log.Printf("run stats:")
	log.Printf("open/pr/stale/migrated: %d/%d/%d/%d ", stats.Processed, stats.PullRequest, stats.Stale, stats.Active)
}
