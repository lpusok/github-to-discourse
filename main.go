package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
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
)

type restoredIssue struct {
	URL    string
	Owner  string
	Repo   string
	IssNum int
	Done   int
	Extra  string
}

type repo struct {
	Owner string
	Name  string
}

type step struct {
	LatestVersionNumber string `json:"latest_version_number"`
	Versions            map[string]map[string]interface{}
}

type spec struct {
	Steps map[string]step
}

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

func prefixWithRunID(str string) string {
	return fmt.Sprintf("[TEST][%s] %s", time.Now().Format(time.RFC3339), str)
}

func saveState(f *os.File, chkpt restoredIssue) error {

	data, err := json.Marshal(chkpt)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		return err
	}

	if err := f.Sync(); err != nil {
		return err
	}

	return nil
}

func continueProcessing(iss *github.Issue, i restoredIssue, f *os.File) (int, string, error) {
	// // continue from next step
	switch i.Done {
	case discourseDone:

		dscURL := i.Extra
		if isStale(iss) {
			if err := comment(iss, fmt.Sprintf(staleTpl, iss.GetUser().GetLogin())); err != nil {
				return commentDone, commentLog, fmt.Errorf("error commenting on github: %s", err)
			}
		} else {
			if err := comment(iss, fmt.Sprintf(activeTpl, iss.GetUser().GetLogin(), dscURL)); err != nil {
				fmt.Printf("error commenting on github: %s", err)
			}
		}

		fallthrough
	case commentDone:
		if err := close(iss); err != nil {
			return commentDone, commentLog, fmt.Errorf("error closing github issue: %s", err)
		}

		fallthrough
	case closeDone:
		if err := lock(iss); err != nil {
			return closeDone, closeLog, fmt.Errorf("error locking github issue: %s", err)

		}

		fallthrough
	case lockDone:
		// // nothing to do
	}
	return lockDone, lockLog, nil
}

func loadRepos(loader string) []repo {
	var baseRepos []repo

	switch loader {
	case "owner":
		l := githubOwnerLoader{client: client}
		baseRepos = l.Load()
	case "steplib":
		l := bitriseSteplibLoader{}
		baseRepos = l.Load()
	case "cherry":
		l := cherryPickLoader{}
		baseRepos = l.Load()
	}

	return baseRepos
}

func loaderMode(src string) string {
	if src == "steplib" {
		return "steplib"
	}

	items := strings.Split(src, ",")

	if _, err := url.Parse(items[0]); err != nil {
		return "owner"
	}

	return "cherry"
}

func main() {

	var mode string
	var repoSrc string
	var chkpt string

	flag.StringVar(&mode, "run-mode", defaultMode, "--runmode=dry|live (dry: only prints what would happen, but modifies nothing)")
	flag.StringVar(&repoSrc, "repos", "", "--repos=[<url>, <url>, ...] | [<org|owner>, <org|owner>, ...] | steplib (cherry pick repos you wish to process)")
	flag.StringVar(&chkpt, "chkpt", "", "--chkpt=checkpoint.log (continue from state stored in checkpoint file)")

	flag.Parse()

	if repoSrc == "" && chkpt == "" {
		fmt.Println("must provide repo source or checkpoint file")
		os.Exit(1)
	}

	baseRepos := loadRepos(loaderMode(repoSrc))

	fmt.Printf("found %d repos, querying open issues", len(baseRepos))
	fmt.Println()
	fmt.Println()

	ferr, err := os.OpenFile("err.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() {
		if err := ferr.Close(); err != nil {
			fmt.Printf("warning: %s", err)
			fmt.Println()
		}
	}()

	if err != nil {
		fmt.Printf("opening state file: %s", err)
		os.Exit(1)
	}

	var stats runStats
	switch mode {
	case "dry":
		issues := githubOpenLoader{}.Load(baseRepos)
		run := dryRun{}

		tbc, err := continueStartedLoader{}.Load()
		if err != nil {
			fmt.Println(fmt.Sprintf("error restoring unfinished issue: %s", err))
			os.Exit(1)
		}
		run.finish(tbc)

		for _, i := range issues {
			fmt.Println(fmt.Sprintf("processing issue %s", i.GetHTMLURL()))
			run.process(i)
			// avoid throttling
			time.Sleep(time.Millisecond + 1000)
		}

		stats = run.stats
	case "live":
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

		run.finish(tbc)

		for _, i := range issues {
			fmt.Println(fmt.Sprintf("processing issue %s", i.GetHTMLURL()))
			err = run.process(i, mode)
			if err != nil {
				fmt.Println(fmt.Sprintf("error processing %s: %s", i.GetHTMLURL(), err))
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
