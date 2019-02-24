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

func getFromStepLib(steplibURL string, githubOrgs []string) (repoURLs []string, err error) {
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
				repoURLs = append(urls, stp.Versions[stp.LatestVersionNumber].Source.Git)
				break
			}
		}
	}

	return repoURLs, nil
}