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
	staffCategory  = 29
	buildIssuesCat = 11
	chkptLog       = "chkpt.log"
	discourseDone  = 1
	commentDone    = 2
	closeDone      = 3
	lockDone       = 4
	maxCount       = 1
	activeTpl      = "Hi %s! We are migrating our GitHub issues to Discourse (https://discuss.bitrise.io/c/issues/build-issues). From now on, you can track this issue at: %s"
	staleTpl       = "Hi %s! We are migrating our GitHub issues to Discourse (https://discuss.bitrise.io/c/issues/build-issues). Because this issue has been inactive for more than three months, we will be closing it. If you feel it is still relevant, please open a ticket on Discourse!"
	defaultMode    = "dry"
)

var (
	client *github.Client
	ctx    context.Context
	tc     *http.Client // todo: check if this can be eliminated
	mode string
	loader string
	chkpt string
)

type runStats struct {
	Processed   int
	Stale       int
	Active      int
	PullRequest int
}

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
		fmt.Println("must provide repo source or checkpoint file")
		os.Exit(1)
	}

	ldr, err := getRepoLoader(loader)
	if err != nil {
		fmt.Println(fmt.Sprintf("error loading repos using %s loader: %s", loader, err))
		os.Exit(1)
	}

	baseRepos, err := ldr.Load()

	fmt.Printf("found %d repos, querying open issues", len(baseRepos))
	fmt.Println()
	fmt.Println()

	chkptf, err := os.OpenFile(chkptLog, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer func() {
		if err := chkptf.Close(); err != nil {
			fmt.Printf("warning: %s", err)
			fmt.Println()
		}
	}()

	tbc, err := continueStartedLoader{}.Load()
	if err != nil {
		fmt.Println(fmt.Sprintf("error restoring unfinished issue: %s", err))
		os.Exit(1)
	}
	issues := githubOpenLoader{}.Load(baseRepos)

	var stats runStats
	switch mode {
	case "dry":
		fmt.Println("running in 'dry' mode")
		run := dryRun{}
		stats, _ = run.run(issues, tbc)
	case "live":
		fmt.Println("running in 'live' mode")
		run := liveRun{
			tc:     tc,
			chkptf: chkptf,
		}
		stats, err = run.run(issues, tbc)
	}

	fmt.Println("==================================")
	fmt.Println("=== Finished processing issues ===")
	fmt.Println("==================================")
	fmt.Println()
	fmt.Println("stale: ", stats.Stale, "active: ", stats.Active, "total processed", stats.Processed)
	fmt.Println("PRs: ", stats.PullRequest)
}
