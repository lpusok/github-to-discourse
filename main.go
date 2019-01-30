package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
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

func main() {
	mode := defaultMode
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	var baseRepos []repo

	switch mode {
	case "test":
		repos, _, err := client.Repositories.List(ctx, "", &github.RepositoryListOptions{
			Affiliation: "owner",
		})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		for _, r := range repos {
			repo := repo{r.GetOwner().GetLogin(), r.GetName()}
			baseRepos = append(baseRepos, repo)
		}
	case "migrate":
		// get spec file
		resp, err := http.Get("https://bitrise-steplib-collection.s3.amazonaws.com/spec.json")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				fmt.Printf("warning: %s", err)
				fmt.Println()
			}
		}()

		// read spec file
		sp, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// unmarshal spec file
		var data spec
		if err := json.Unmarshal(sp, &data); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// process steps
		for _, stp := range data.Steps {

			// get latest version for step
			url, ok := stp.Versions[stp.LatestVersionNumber]["source_code_url"].(string)
			if !ok {
				fmt.Println("could not convert json data")
				os.Exit(1)
			}

			orgs := []string{
				"bitrise-steplib",
				"bitrise-io",
				"bitrise-core",
				"bitrise-community",
				"bitrise-tools",
				"bitrise-docker",
				"bitrise-samples",
			}

			// filter to our repositories
			for _, o := range orgs {

				fragments := strings.Split(url, "/")
				name := fragments[len(fragments)-1]
				owner := fragments[len(fragments)-2]
				if owner == o {
					repo := repo{owner, name}
					baseRepos = append(baseRepos, repo)
					break
				}
			}

		}
		fmt.Printf("found %d repos, querying open issues", len(baseRepos))
		fmt.Println()
		fmt.Println()

	}

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

	// get issues for repositories
	opts := github.IssueListByRepoOptions{
		State: "open",
	}

	var stats *runStats
	switch mode {
	case "test", "migrate":
		for j, r := range baseRepos {
			fmt.Println()
			fmt.Println(strings.Repeat("=", 80))
			fmt.Println(fmt.Sprintf("  processing repo (%d/%d): %s", len(baseRepos), j, r.Name))
			fmt.Println(strings.Repeat("=", 80))
			fmt.Println()

			issues, _, err := client.Issues.ListByRepo(ctx, r.Owner, r.Name, &opts)
			if err != nil {
				fmt.Printf("error getting issues list: %s", err)
				fmt.Println()
			}

			stats, err = process(tc, issues, f, mode)
			if err != nil {
				fmt.Println(fmt.Sprintf("mode: %s: %s", mode, err))
				os.Exit(1)
			}

		}
	case "continue":
		// load state file
		content, err := ioutil.ReadFile(stateFile)
		if err != nil {
			fmt.Println(fmt.Sprintf("could not read restore file: %s", err))
			os.Exit(1)
		}

		// get (issue -> last state) map
		lines := strings.Split(string(content), "\n")
		issueStates := make(map[string]*restoredIssue)
		for _, l := range lines {
			if len(l) == 0 {
				continue
			}

			// parse line
			fields := strings.Split(l, " ")

			url := fields[0]
			fragments := strings.Split(url, "/")

			owner := fragments[3]
			repo := fragments[4]
			num, err := strconv.Atoi(fragments[6])
			if err != nil {
				fmt.Println(fmt.Sprintf("could not read stored state: %s", err))
			}

			done, err := strconv.Atoi(fields[1])
			if err != nil {
				fmt.Println(fmt.Sprintf("could not read stored state: %s", err))
			}

			extra := ""
			if done == discourseDone {
				extra = fields[2]
			}

			iss := restoredIssue{
				Owner:  owner,
				Repo:   repo,
				IssNum: num,
				URL:    url,
				Done:   done,
				Extra:  extra,
			}

			// populate map: update or insert value
			if i, ok := issueStates[iss.URL]; ok {
				if done > i.Done {
					i.Done = done
				}
			} else {
				issueStates[url] = &iss
			}

		}

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

			// // continue from next step
			state := i.Done + 1
			switch i.Done {
			case discourseDone:

				dscURL := i.Extra
				if isStale(iss) {
					if err := comment(iss, fmt.Sprintf(staleTpl, iss.GetUser().GetLogin())); err != nil {
						fmt.Printf("error commenting on github: %s", err)
						continue
					}
				} else {
					if err := comment(iss, fmt.Sprintf(activeTpl, iss.GetUser().GetLogin(), dscURL)); err != nil {
						fmt.Printf("error commenting on github: %s", err)
						continue
					}
				}

				if err := saveState(f, iss, commentDone, "", commentLog); err != nil {
					fmt.Printf("fatal error saving state %d: %s", state, err)
					os.Exit(1)
				}
				fallthrough
			case commentDone:
				if err := close(iss); err != nil {
					fmt.Printf("error closing github issue: %s", err)
					continue
				}

				if err := saveState(f, iss, closeDone, "", closeLog); err != nil {
					fmt.Printf("fatal error saving state %d: %s", state, err)
					os.Exit(1)
				}
				fallthrough
			case closeDone:
				if err := lock(iss); err != nil {
					fmt.Printf("error locking github issue: %s", err)
					continue
				}
				if err := saveState(f, iss, lockDone, "", lockLog); err != nil {
					fmt.Printf("fatal error saving state %d: %s", state, err)
					os.Exit(1)
				}
				fallthrough
			case lockDone:
				// // nothing to do
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
