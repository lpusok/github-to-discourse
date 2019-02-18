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

	var baseRepos []repo
	var err error
	switch loader {
	case "owner":
		baseRepos, err = githubOwnerLoader{client: client}.Load()
	case "steplib":
		baseRepos, err = bitriseSteplibLoader{}.Load()
	case "cherry":
		baseRepos, err = cherryPickLoader{}.Load()
	default:
		baseRepos, err = nil, fmt.Errorf("unkown loader %s", loader)
	}
	if err != nil {
		fmt.Println("error loading repos using %s loader: %s", loader, err)
		os.Exit(1)
	}

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

		stats, err = run.run(issues, tbc)
	}

	fmt.Println("==================================")
	fmt.Println("=== Finished processing issues ===")
	fmt.Println("==================================")
	fmt.Println()
	fmt.Println("stale: ", stats.Stale, "active: ", stats.Active, "total processed", stats.Processed)
	fmt.Println("PRs: ", stats.PullRequest)
}
