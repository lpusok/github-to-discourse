package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

const (
	staffCategory  = 29
	buildIssuesCat = 11
	chkptLog       = "chkpt.log"
	discourseDone  = 1
	discourseLog   = "      Migrated to Discourse: %s"
	commentDone    = 2
	commentLog     = "      Commented on issue"
	closeDone      = 3
	closeLog       = "      Closed GitHub issue"
	lockDone       = 4
	lockLog        = "      Locked GitHub issue"
	maxCount       = 1
	activeTpl      = "Hi %s! We are migrating our GitHub issues to Discourse (https://discuss.bitrise.io/c/issues/build-issues). From now on, you can track this issue at: %s"
	staleTpl       = "Hi %s! We are migrating our GitHub issues to Discourse (https://discuss.bitrise.io/c/issues/build-issues). Because this issue has been inactive for more than three months, we will be closing it. If you feel it is still relevant, please open a ticket on Discourse!"
	defaultMode    = "dry"
)

var (
	client *github.Client
	ctx    context.Context
	tc     *http.Client // todo: check if this can be eliminated
	runID  string
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

}

func main() {

	var mode string
	var loader string
	var chkpt string

	flag.StringVar(&mode, "run-mode", defaultMode, "--runmode=dry|live (dry: only prints what would happen, but modifies nothing)")
	flag.StringVar(&loader, "repo-loader", "cherry", "--repo-loader=cherry|owner|steplib (repo loader to use to process arguments)")
	flag.StringVar(&chkpt, "chkpt", "", "--chkpt=checkpoint.log (continue from state stored in checkpoint file)")
	flag.StringVar(&runID, "run-id", "", "--run-id=<string> (created resources will have 'myrunid' baked into title for easier identification)")
	flag.IntVar(&discourseCategoryID, "discourse-category-id", staffCategory, "--discourse-category-id=<int> (discourse category to post topics to)")

	flag.Parse()

	if loader == "" && chkpt == "" {
		fmt.Println("must provide repo source or checkpoint file")
		os.Exit(1)
	}

	baseRepos := loadRepos(loader)

	fmt.Printf("found %d repos, querying open issues", len(baseRepos))
	fmt.Println()
	fmt.Println()

	if _, err := os.Stat(chkptLog); os.IsNotExist(err) {
		chkptf, err := os.Create(chkptLog)
		if err != nil {
			fmt.Println("error: create checkpoint file: %s")
			os.Exit(1)
		}
		defer chkptf.Close()
	}

	var stats runStats
	switch mode {
	case "dry":
		fmt.Println("running in 'dry' mode")
		issues := githubOpenLoader{}.Load(baseRepos)
		run := dryRun{}

		tbc, err := continueStartedLoader{}.Load()
		if err != nil {
			fmt.Println(fmt.Sprintf("error restoring unfinished issue: %s", err))
			os.Exit(1)
		}
		if tbc != nil {
			run.finish(tbc)
		}

		for _, i := range issues {
			fmt.Println(fmt.Sprintf("processing issue %s", i.GetHTMLURL()))
			run.process(i)
			// avoid throttling
			time.Sleep(time.Millisecond + 1000)
		}

		stats = run.stats
	case "live":
		fmt.Println("running in 'live' mode")
		fchkpt, err := os.OpenFile(chkptLog, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		defer func() {
			if err := fchkpt.Close(); err != nil {
				fmt.Printf("warning: %s", err)
				fmt.Println()
			}
		}()

		issues := githubOpenLoader{}.Load(baseRepos)
		run := liveRun{
			tc:     tc,
			chkptf: fchkpt,
		}

		tbc, err := continueStartedLoader{}.Load()
		if err != nil {
			fmt.Println(fmt.Sprintf("error restoring unfinished issue: %s", err))
			os.Exit(1)
		}

		if tbc != nil {
			run.finish(tbc)
		}

		for _, i := range issues {
			fmt.Println(fmt.Sprintf("processing issue %s", i.GetHTMLURL()))
			err = run.process(i)
			if err != nil {
				fmt.Println(fmt.Sprintf("error processing GET %s: %s", i.GetHTMLURL(), err))
				os.Exit(1)
			}
			// avoid throttling
			time.Sleep(time.Millisecond + 1000)
		}
		stats = run.stats
	}

	fmt.Println("==================================")
	fmt.Println("=== Finished processing issues ===")
	fmt.Println("==================================")
	fmt.Println()
	fmt.Println("stale: ", stats.Stale, "active: ", stats.Active, "total processed", stats.Processed)
	fmt.Println("PRs: ", stats.PullRequest)
}
