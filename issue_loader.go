package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/google/go-github/github"
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

func (il unfinishedIssueLoader) Load() (*github.Issue, error) {
	content, err := ioutil.ReadFile(chkptLog)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint file: %s", err)
	}
	if len(content) == 0 {
		fmt.Println("checkpoint file empty")
		return nil, nil
	}

	var restored restoredIssue
	if err := json.Unmarshal([]byte(content), &restored); err != nil {
		return nil, err
	}

	issue, resp, err := client.Issues.Get(ctx, restored.Owner, restored.Repo, restored.IssNum)
	if err != nil {
		fmt.Println(fmt.Sprintf("fetch %s from github: %s", restored.Repo, err))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s from github: %s", restored.Repo, resp.Status)
	}

	return issue, nil
}
