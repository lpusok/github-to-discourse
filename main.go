package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
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

func process(tc *http.Client, i *github.Issue, f *os.File, mode string, stats *runStats) error {
	stats = &runStats{}

	// avoid throttling
	time.Sleep(time.Millisecond + 1000)

	// skip if PR
	if i.IsPullRequest() {
		stats.PullRequest++
		printSkipPR(i.GetNumber(), i.GetHTMLURL())
		fmt.Println()
		return nil
	}

	// short circuit if reached processing limit
	if stats.Processed == maxCount {
		printMaxCountReached()
		fmt.Println()
		return nil
	}

	fmt.Println()

	if !isStale(i) {

		printIssueLog("Issue is active")
		fmt.Println()
		stats.Active++

		// discourse
		url, err := discourse(i, mode)
		if err != nil {
			return err
		}

		if err := saveState(f, i, discourseDone, url, fmt.Sprintf(discourseLog, url)); err != nil {
			return fmt.Errorf("process: %s", err)
		}

		// comment
		if err := comment(i, fmt.Sprintf(activeTpl, i.GetUser().GetLogin(), url)); err != nil {
			return err
		}
	} else {

		stats.Stale++
		printIssueLog("Issue is stale")
		fmt.Println()

		// comment
		if err := comment(i, fmt.Sprintf(staleTpl, i.GetUser().GetLogin())); err != nil {
			return err
		}
	}

	if err := saveState(f, i, commentDone, "", commentLog); err != nil {
		return fmt.Errorf("process: %s", err)
	}

	// close
	if err := close(i); err != nil {
		return err
	}

	if err := saveState(f, i, closeDone, "", closeLog); err != nil {
		return fmt.Errorf("process: %s", err)
	}

	// lock
	if err := lock(i); err != nil {
		return err
	}

	if err := saveState(f, i, lockDone, "", lockLog); err != nil {
		return fmt.Errorf("process: %s", err)
	}
	stats.Processed++

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
	mode := defaultMode
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	var repoSrc string
	var chkpt string

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

	fchkpt, err := os.OpenFile(chkptLog, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
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

	stats := runStats{}
	switch mode {
	case "test", "migrate":
		issues := githubOpenLoader{}.Load(baseRepos)
		for k, i := range issues {
			printIssueHeader(len(issues), k+1, i.GetNumber(), i.GetHTMLURL())
			err = process(tc, i, fchkpt, mode, &stats)
			if err != nil {
				printIssueLog(err.Error())
				fmt.Println()
				os.Exit(1)
			}
		}

	case "continue":
		issueStates := continueStartedLoader{}.Load()
		k := 0
		for _, i := range issueStates {

			if k == maxCount {
				printMaxCountReached()
				break
			}

			// // get specific issue
			iss, resp, err := client.Issues.Get(ctx, i.Owner, i.Repo, i.IssNum)
			if err != nil {
				printIssueLog(fmt.Sprintf("error getting issue: %s", err))
				continue
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					printIssueLog(fmt.Sprintf("could not read response body: %s", err))
					continue
				}
				printIssueLog(fmt.Sprintf("api error: %s", body))
				continue
			}

			printIssueHeader(len(issueStates), len(issueStates), i.IssNum, i.URL)

			state, logmsg, err := continueProcessing(iss, *i, fchkpt)

			if err != nil {
				fmt.Println(err)
			}

			if err := saveState(fchkpt, iss, state, "", logmsg); err != nil {
				fmt.Println(fmt.Printf("fatal error saving state %d: %s", state, err))
				os.Exit(1)
			}
			k++
		}
	}

	fmt.Println("==================================")
	fmt.Println("=== Finished processing issues ===")
	fmt.Println("==================================")
	fmt.Println()
	fmt.Println("stale: ", stats.Stale, "active: ", stats.Active, "total processed", stats.Processed)
	fmt.Println("PRs: ", stats.PullRequest)
}
