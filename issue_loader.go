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

func (il continueStartedLoader) Load() map[string]*restoredIssue {
	// load state file
	content, err := ioutil.ReadFile(chkptLog)
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

	return issueStates
}
