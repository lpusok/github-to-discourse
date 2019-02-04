package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/go-github/github"
)

type repoLoader interface {
	Load() []repo
}

type githubOwnerLoader struct {
	client *github.Client
}

type bitriseSteplibLoader struct{}

type cherryPickLoader struct{}

func (l githubOwnerLoader) Load() []repo {
	var baseRepos []repo
	repos, _, err := l.client.Repositories.List(ctx, "", &github.RepositoryListOptions{
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

	return baseRepos
}

func (l bitriseSteplibLoader) Load() []repo {
	var baseRepos []repo
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

	return baseRepos
}

func (l cherryPickLoader) Load() []repo {
	arg := flag.Lookup("repos")
	if arg == nil {
		fmt.Println(fmt.Sprintf("`repos` argument missing"))
		os.Exit(1)
	}

	repos := make([]repo, 0)
	for _, repoURL := range strings.Split(arg.Value.String(), ",") {
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

	return repos
}
