package main

import (
	"context"
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
	stateFile      = "data.txt"
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
	defaultMode    = "test"
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

func saveState(f *os.File, i *github.Issue, state int, extra string, logmsg string) error {
	line := fmt.Sprintf("%s %d %s\n", i.GetHTMLURL(), state, extra)

	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("save state: %s", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("save state: sync file: %s", err)
	}

	fmt.Println()
	fmt.Println(fmt.Sprintf(logmsg))

	return nil
}

func process(tc *http.Client, issues []*github.Issue, f *os.File, mode string) (stats *runStats, err error) {
	stats = &runStats{}
	for k, i := range issues {
		// avoid throttling
		time.Sleep(time.Millisecond + 1000)

		printIssueHeader(len(issues), k+1, i.GetNumber(), i.GetHTMLURL())

		// skip if PR
		if i.IsPullRequest() {
			stats.PullRequest++
			printSkipPR(i.GetNumber(), i.GetHTMLURL())
			fmt.Println()
			continue
		}

		// short circuit if reached processing limit
		if stats.Processed == maxCount {
			printMaxCountReached()
			fmt.Println()
			break
		}

		fmt.Println()

		if !isStale(i) {

			printIssueLog("Issue is active")
			fmt.Println()
			stats.Active++

			// discourse
			url, err := discourse(i, mode)
			if err != nil {
				printIssueLog(err.Error())
				fmt.Println()
				continue
			}

			if err := saveState(f, i, discourseDone, url, fmt.Sprintf(discourseLog, url)); err != nil {
				return nil, fmt.Errorf("process: %s", err)
			}

			// comment
			if err := comment(i, fmt.Sprintf(activeTpl, i.GetUser().GetLogin(), url)); err != nil {
				printIssueLog(err.Error())
				fmt.Println()
				continue
			}
		} else {

			stats.Stale++
			printIssueLog("Issue is stale")
			fmt.Println()

			// comment
			if err := comment(i, fmt.Sprintf(staleTpl, i.GetUser().GetLogin())); err != nil {
				printIssueLog(err.Error())
				fmt.Println()
				continue
			}
		}

		if err := saveState(f, i, commentDone, "", commentLog); err != nil {
			return nil, fmt.Errorf("process: %s", err)
		}

		// close
		if err := close(i); err != nil {
			printIssueLog(err.Error())
			fmt.Println()
			continue
		}

		if err := saveState(f, i, closeDone, "", closeLog); err != nil {
			return nil, fmt.Errorf("process: %s", err)
		}

		// lock
		if err := lock(i); err != nil {
			printIssueLog(err.Error())
			fmt.Println()
			continue
		}

		if err := saveState(f, i, lockDone, "", lockLog); err != nil {
			return nil, fmt.Errorf("process: %s", err)
		}
		stats.Processed++
	}

	return stats, nil
}

func loadRepos(loader string) []repo {
	var baseRepos []repo

	switch loader {
	case "test":
		l := githubOwnerLoader{client: client}
		baseRepos = l.Load()
	case "migrate":
		l := bitriseSteplibLoader{}
		baseRepos = l.Load()
	}

	return baseRepos
}

func main() {
	mode := defaultMode
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	baseRepos := loadRepos(mode)

	fmt.Printf("found %d repos, querying open issues", len(baseRepos))
	fmt.Println()
	fmt.Println()

	f, err := os.OpenFile(stateFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("warning: %s", err)
			fmt.Println()
		}
	}()

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

	var stats *runStats
	switch mode {
	case "test", "migrate":
		issues := githubOpenLoader{}.Load(baseRepos)
		stats, err = process(tc, issues, f, mode)
		if err != nil {
			fmt.Println(fmt.Sprintf("mode: %s: %s", mode, err))
			os.Exit(1)
		}
	case "continue":
		continueStartedLoader{}.Load(f)
	}

	fmt.Println("==================================")
	fmt.Println("=== Finished processing issues ===")
	fmt.Println("==================================")
	fmt.Println()
	fmt.Println("stale: ", stats.Stale, "active: ", stats.Active, "total processed", stats.Processed)
	fmt.Println("PRs: ", stats.PullRequest)
}
