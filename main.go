package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/google/go-github/github"
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
}

func main() {

	flag.Parse()

	if loader == "" && chkpt == "" {
		fmt.Println("error: must provide repo source or checkpoint file")
		os.Exit(1)
	}

	ldr, err := getRepoLoader(loader)
	if err != nil {
		fmt.Println(fmt.Sprintf("error loading repos using %s loader: %s", loader, err))
		os.Exit(1)
	}

	baseRepos, err := ldr.Load()

	unfinished, err := unfinishedIssueLoader{}.Load()
	if err != nil {
		fmt.Println(fmt.Sprintf("error restoring unfinished issue: %s", err))
		os.Exit(1)
	}
	issues := openGithubIssueLoader{}.Load(baseRepos)

	runMode, err := getRunMode(mode)
	if err != nil {
		fmt.Println(fmt.Sprintf("error selecting run mode: %s", err))
		os.Exit(1)
	}

	stats, err := runMode.run(issues, unfinished)
	if err != nil {
		fmt.Println(fmt.Sprintf("error running in %s mode: %s", mode, err))
		os.Exit(1)
	}

	fmt.Println("finished processing issues!")
	fmt.Println("run stats:")
	fmt.Println(fmt.Sprintf("open/pr/stale/migrated: %d/%d/%d/%d ", stats.Processed, stats.PullRequest, stats.Stale, stats.Active))
}
