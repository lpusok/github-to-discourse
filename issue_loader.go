package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"github.com/bitrise-io/go-utils/log"
)

type openGithubIssueLoader struct{}

type unfinishedIssueLoader struct{}

func (il openGithubIssueLoader) Load(baseRepos []repo) []*github.Issue {
	var all []*github.Issue
	// get issues for repositories
	opts := github.IssueListByRepoOptions{
		State: "open",
	}
	for _, r := range baseRepos {

		issues, _, err := client.Issues.ListByRepo(ctx, r.Owner, r.Name, &opts)
		if err != nil {
			fmt.Printf("fetch issues: %s", err)
			fmt.Println()
			continue
		}

		all = append(all, issues...)

	}
	return all
}

func (il unfinishedIssueLoader) Load() (*github.Issue, int, error) {
	content, err := ioutil.ReadFile(chkptLog)
	if err != nil {
		return nil, 0, fmt.Errorf("read checkpoint file: %s", err)
	}
	if len(content) == 0 {
		log.Warnf("checkpoint file empty")
		return nil, 0, nil
	}

	var restored restoredIssue
	if err := json.Unmarshal([]byte(content), &restored); err != nil {
		return nil, 0, err
	}

	fragments := strings.Split(restored.URL, "/")
	owner := fragments[len(fragments)-4]
	repo := fragments[len(fragments)-3]
	issNum, err := strconv.Atoi(fragments[len(fragments)-1])
	if err != nil {
		return nil, 0, fmt.Errorf("parse issue details from url %s: %s", restored.URL, err)
	}

	issue, resp, err := client.Issues.Get(ctx, owner, repo, issNum)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch %s from github: %s", restored.URL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("fetch %s from github: %s", restored.URL, resp.Status)
	}

	return issue, restored.Done, nil
}
