package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	stepmanModels "github.com/bitrise-io/stepman/models"
	"github.com/bitrise-io/go-utils/log"
	"github.com/google/go-github/github"
)

type repoURL string

func (url repoURL) owner() {
	fragments := strings.Split(string(url), "/")
	return fragments[len(fragments)-2]
}

func (r repoURL) name() {
	fragments := strings.Split(string(url), "/")
	return strings.TrimSuffix(fragments[len(fragments)-1], ".git")
}

func getFromStepLib(steplibURL string, githubOrgs []string) ([]repoURL, error) {
	var urls []repoURL
	// get spec file
	resp, err := http.Get(steplibURL)
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
	var data stepmanModels.StepCollectionModel
	if err := json.Unmarshal(sp, &data); err != nil {
		return nil, fmt.Errorf("unmarshal steplib json %s: %s", string(sp), err)
	}

	// process steps
	for _, stp := range data.Steps {
		// filter to our repositories
		for _, o := range orgs {
			if owner == o {
				urls = append(urls, stp.Versions[stp.LatestVersionNumber].Source.Git)
				break
			}
		}
	}

	return urls, nil
}
