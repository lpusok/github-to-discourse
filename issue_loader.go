package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
)

type githubOpenLoader struct{}

type continueStartedLoader struct{}

func (il githubOpenLoader) Load(baseRepos []repo) []*github.Issue {
	var all []*github.Issue
	// get issues for repositories
	opts := github.IssueListByRepoOptions{
		State: "open",
	}
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

		all = append(all, issues...)

	}
	return all
}

func (il continueStartedLoader) Load(f *os.File) []*github.Issue {
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

	return nil
}
