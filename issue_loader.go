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

func getOpenIssues(repoURLs []repoURLS) []*github.Issue {
	var all []*github.Issue
	// get issues for repositories
	opts := github.IssueListByRepoOptions{
		State: "open",
	}
	for _, url := range repoURLs {

		issues, _, err := client.Issues.ListByRepo(ctx, url.owner(), url.name(), &opts)
		if err != nil {
			fmt.Printf("fetch issues: %s", err)
			fmt.Println()
			continue
		}

		all = append(all, issues...)

	}
	return all
}
