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
)

type RestoredIssue struct {
	URL    string
	Owner  string
	Repo   string
	IssNum int
	Done   int
	Extra  string
}

type Repo struct {
	Owner string
	Name  string
}

type Step struct {
	LatestVersionNumber string `json:"latest_version_number"`
	Versions            map[string]map[string]interface{}
}

type Spec struct {
	Steps map[string]Step
}

func prefixWithRunID(str string) string {
	return fmt.Sprintf("[TEST][%s] %s", time.Now().Format(time.RFC3339), str)
}

func process(issues []*github.Issue, f *os.File) (c, staleCount, activeCount, cPR int) {
	for k, i := range issues {
		// avoid throttling
		time.Sleep(time.Millisecond + 1000)

		printIssueHeader(len(issues), k, i.GetNumber(), i.GetHTMLURL())

		// skip if PR
		if i.IsPullRequest() {
			cPR++
			printSkipPR(i.GetNumber(), i.GetHTMLURL())
			fmt.Println()
			continue
		}

		// short circuit if reached processing limit
		if c == maxCount {
			printMaxCountReached()
			fmt.Println()
			break
		}

		fmt.Println()

		if !isStale(i) {

			printIssueLog("Issue is active")
			fmt.Println()
			activeCount++

			// discourse
			url, err := discourse(i)
			if err != nil {
				printIssueLog(err.Error())
				fmt.Println()
				continue
			}

			saveState(f, i, discourseDone, url, fmt.Sprintf(discourseLog, url))

			// comment
			err = comment(i, fmt.Sprintf(activeTpl, url))
			if err != nil {
				printIssueLog(err.Error())
				fmt.Println()
				continue
			}
		} else {

			staleCount++
			printIssueLog("Issue is stale")
			fmt.Println()

			// comment
			err := comment(i, fmt.Sprintf(staleTpl))
			if err != nil {
				printIssueLog(err.Error())
				fmt.Println()
				continue
			}
		}

		saveState(f, i, commentDone, "", commentLog)

		// close
		err := close(i)
		if err != nil {
			printIssueLog(err.Error())
			fmt.Println()
			continue
		}

		saveState(f, i, closeDone, "", closeLog)

		// lock
		err = lock(i)
		if err != nil {
			printIssueLog(err.Error())
			fmt.Println()
			continue
		}

		saveState(f, i, lockDone, "", lockLog)
		c++
	}

	return c, staleCount, activeCount, cPR
}

func main() {
	mode := os.Args[1:][0]
	var baseRepos []Repo

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	switch mode {
	case "test":
		repos, _, _ := client.Repositories.List(ctx, "", &github.RepositoryListOptions{
			Affiliation: "owner",
		})
		for _, r := range repos {
			repo := Repo{r.GetOwner().GetLogin(), r.GetName()}
			baseRepos = append(baseRepos, repo)
		}
	case "migrate":
		// get spec file
		resp, err := http.Get("https://bitrise-steplib-collection.s3.amazonaws.com/spec.json")
		if err != nil {
			panic(err)
		}

		// read spec file
		spec, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// unmarshal spec file
		var data Spec
		err = json.Unmarshal(spec, &data)
		if err != nil {
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
				if strings.Contains(url, o) {
					fragments := strings.Split(url, "/")
					name := fragments[len(fragments)-1]
					owner := fragments[len(fragments)-2]
					repo := Repo{owner, name}
					baseRepos = append(baseRepos, repo)
				}
			}

		}
		fmt.Printf("found %d repos, querying open issues", len(baseRepos))
		fmt.Println()
		fmt.Println()

	}

	// c, staleCount, activeCount, cPR := 0, 0, 0, 0
	var err error
	f, err := os.OpenFile(stateFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	ferr, err := os.OpenFile("err.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	defer f.Close()
	defer ferr.Close()
	if err != nil {
		fmt.Printf("opening state file: %s", err)
		os.Exit(1)
	}

	// get issues for repositories
	opts := github.IssueListByRepoOptions{
		State: "open",
	}

	var c, staleCount, activeCount, cPR int
	switch mode {
	case "test", "migrate":
		for j, r := range baseRepos {
			fmt.Println()
			fmt.Println(strings.Repeat("=", 80))
			fmt.Println(fmt.Sprintf("  processing repo (%d/%d): %s", len(baseRepos), j, r.Name))
			fmt.Println(strings.Repeat("=", 80))
			fmt.Println()

			issues, _, _ := client.Issues.ListByRepo(ctx, r.Owner, r.Name, &opts)

			c, staleCount, activeCount, cPR = process(issues, f)

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
		issueStates := make(map[string]*RestoredIssue)
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
			num64, err := strconv.ParseInt(fragments[6], 10, 8)
			if err != nil {
				fmt.Println(fmt.Sprintf("could not read stored state: %s", err))
			}
			num := int(num64)

			done64, err := strconv.ParseInt(fields[1], 10, 8)
			if err != nil {
				fmt.Println(fmt.Sprintf("could not read stored state: %s", err))
			}
			done := int(done64)

			extra := ""
			if done == discourseDone {
				extra = fields[2]
			}

			iss := RestoredIssue{
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
			switch i.Done {
			case discourseDone:

				// dscURL := i.Extra
				if isStale(iss) {
					// comment(iss, fmt.Sprintf(staleTpl, iss.GetUser().GetLogin()))
				} else {
					// comment(iss, fmt.Sprintf(activeTpl, iss.GetUser().GetLogin(), dscURL))
				}
				saveState(f, iss, commentDone, "", commentLog)
				fallthrough
			case commentDone:
				// close(iss)
				saveState(f, iss, closeDone, "", closeLog)
				fallthrough
			case closeDone:
				// lock(iss)
				saveState(f, iss, lockDone, "", lockLog)
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
	fmt.Println("stale: ", staleCount, "active: ", activeCount, "total processed", c)
	fmt.Println("PRs: ", cPR)
}
