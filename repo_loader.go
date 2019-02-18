package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/github"
)

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

type repoLoader interface {
	Load() ([]repo, error)
}

type githubOwnerLoader struct {
	client *github.Client
}

type bitriseSteplibLoader struct{}

type cherryPickLoader struct{}

func (l githubOwnerLoader) Load() ([]repo, error) {
	var baseRepos []repo
	repos, _, err := l.client.Repositories.List(ctx, "", &github.RepositoryListOptions{
		Affiliation: "owner",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch repositories owned by authenticated user: %s", err)
	}

	for _, r := range repos {
		repo := repo{r.GetOwner().GetLogin(), r.GetName()}
		baseRepos = append(baseRepos, repo)
	}

	return baseRepos, nil
}

func (l bitriseSteplibLoader) Load() ([]repo, error) {
	var baseRepos []repo
	// get spec file
	resp, err := http.Get("https://bitrise-steplib-collection.s3.amazonaws.com/spec.json")
	if err != nil {
		return nil, fmt.Errorf("fetch steplib json: %s", err)
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
		return nil, fmt.Errorf("read steplib json: %s", err)
	}

	// unmarshal spec file
	var data spec
	if err := json.Unmarshal(sp, &data); err != nil {
		return nil, fmt.Errorf("unmarshal steplib json %s: %s", string(sp), err)
	}

	// process steps
	for _, stp := range data.Steps {

		// get latest version for step
		url, ok := stp.Versions[stp.LatestVersionNumber]["source_code_url"].(string)
		if !ok {
			return nil, fmt.Errorf("get source code url: %s", err)
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

	return baseRepos, nil
}

func (l cherryPickLoader) Load() ([]repo, error) {
	repos := make([]repo, 0)
	for _, repoURL := range flag.Args() {
		if _, err := url.Parse(repoURL); err != nil {
			// todo: log warning using bitrise log pkg
			fmt.Println(fmt.Sprintf("repo url %s invalid: %s", repoURL, err))
			continue
		}

		fragments := strings.Split(repoURL, "/")
		repos = append(repos, repo{
			Name:  fragments[len(fragments)-1],
			Owner: fragments[len(fragments)-2],
		})
	}

	return repos, nil
}
