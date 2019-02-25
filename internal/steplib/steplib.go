package steplib

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	stepmanModels "github.com/bitrise-io/stepman/models"
	"github.com/bitrise-io/go-utils/log"
)

func LoadRepos(steplibURL string, fromOrgs []string) (repoURLs []string, err error) {
	// get spec file
	resp, err := http.Get(steplibURL)
	if err != nil {
		return nil, fmt.Errorf("fetch steplib json: %s", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Warnf("close response body: %s", err)
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
		for _, o := range fromOrgs {
			substr := strings.SplitAfter(o, "github.com/")[1]
			owner := strings.Split(substr, "/")[0]
			if owner == o {
				repoURLs = append(repoURLs, stp.Versions[stp.LatestVersionNumber].Source.Git)
				break
			}
		}
	}

	return repoURLs, nil
}
