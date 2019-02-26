package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/log"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var (
	client *github.Client
	ctx    context.Context
	tc     *http.Client
)

func init() {
	ctx = context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc = oauth2.NewClient(ctx, ts)
	client = github.NewClient(tc)
}

func GetOpenIssues(repoURLs []string) []*github.Issue {
	var all []*github.Issue
	opts := github.IssueListByRepoOptions{
		State: "open",
	}
	for _, url := range repoURLs {
		fragments := strings.Split(string(url), "/")
		owner := fragments[len(fragments)-2]
		name := strings.TrimSuffix(fragments[len(fragments)-1], ".git")
		
		issues, resp, err := client.Issues.ListByRepo(ctx, owner, name, &opts)
		if err != nil {
			log.Warnf("fetch issues from %s: %s", url, err)
			continue
		}

		if resp.Response.StatusCode != 200 {
			log.Warnf("fetch issues from %s: %s", url, resp.Response.Status)
			continue
		}

		all = append(all, issues...)

	}
	return all
}

func IsStale(i *github.Issue) bool {
	threeMonthsAgo := time.Now().AddDate(0, -3, 0)
	return i.GetUpdatedAt().Before(threeMonthsAgo)
}

func PostComment(i *github.Issue, comment string) error {
	commentPayload := map[string]interface{}{
		"body": comment,
	}

	data, err := json.Marshal(commentPayload)
	if err != nil {
		return fmt.Errorf("marshal %s: %s", commentPayload, err)
	}

	req, err := http.NewRequest("POST", i.GetCommentsURL(), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("create POST %s request with payload %s: %s", i.GetCommentsURL(), string(data), err)
	}

	resp, err := tc.Do(req)
	if err != nil {
		return fmt.Errorf("send POST %s request with payload %s: %s", i.GetCommentsURL(), string(data), err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("warning: close response body: %s", err)
		}
	}()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %s", err)
	}
	if resp.StatusCode != 201 {
		return fmt.Errorf("api error: POST %s %s: %s %s", i.GetCommentsURL(), data, resp.Status, body)
	}

	return nil
}

func Close(i *github.Issue) error {
	payload := map[string]interface{}{
		"state": "closed",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("could not marshal %s: %s", payload, err)
	}

	request, err := http.NewRequest("PATCH", i.GetURL(), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("could not create request: %s", err)
	}

	resp, err := tc.Do(request)
	if err != nil {
		return fmt.Errorf("could not send request: %s", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("warning: could not close response body: %s", err)
		}
	}()

	if err != nil {
		return fmt.Errorf("error sending request: %s", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read response body: %s", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("api error for payload %s: %s", payload, body)
	}

	return nil
}

func Lock(i *github.Issue) error {
	// // lock
	url := fmt.Sprintf("%s/lock", i.GetURL())
	request, err := http.NewRequest("PUT", url, bytes.NewBuffer([]byte{}))
	request.Header.Add("Content-Length", "0")
	if err != nil {
		return fmt.Errorf("could not create request: %s", err)
	}

	resp, err := tc.Do(request)
	if err != nil {
		return fmt.Errorf("could not send request: %s", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("warning: could not close response body: %s", err)
		}
	}()

	if err != nil {
		return fmt.Errorf("error sending request: %s", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read response body: %s", err)
	}
	if resp.StatusCode != 204 {
		return fmt.Errorf("api error: %s", body)
	}

	return nil
}
