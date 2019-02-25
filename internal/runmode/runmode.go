package runmode

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/go-github/github"
	"github.com/bitrise-io/go-utils/log"

	"github.com/lszucs/github-to-discourse/internal/github"
	"github.com/lszucs/github-to-discourse/internal/discourse"
)

const (
	discourseDone = iota
	commentDone
	closeDone
	lockDone
)

const (
	activeTpl = "Hi %s! We are migrating our GitHub issues to Discourse (https://discuss.bitrise.io/c/issues/build-issues). From now on, you can track this issue at: %s"
	staleTpl  = "Hi %s! We are migrating our GitHub issues to Discourse (https://discuss.bitrise.io/c/issues/build-issues). Because this issue has been inactive for more than three months, we will be closing it. If you feel it is still relevant, please open a ticket on Discourse!"
)

var runID string

func init() {
	flag.StringVar(&runID, "run-id", "", "--run-id=<string> (created resources will have 'myrunid' baked into title for easier identification)")
}

func DryRun(issues []*github.Issue, stats *runStats) error {
	for _, i := range issues {
		log.Printf("processing issue %s", i.GetHTMLURL())
		if i.IsPullRequest() {
			stats.PullRequest++
			fmt.Println(fmt.Sprintf("skip %s: is pull request", i.GetHTMLURL()))
			continue
		}
	
		if !github.IsStale(i) {
			stats.Active++
			fmt.Println(fmt.Sprintf("%s is active", i.GetHTMLURL()))
		} else {
			run.stats.Stale++
			fmt.Println(fmt.Sprintf("%s is stale", i.GetHTMLURL()))
		}
		stats.Processed++
		time.Sleep(time.Millisecond + 1000)
	}

	return nil
}

func LiveRun(issues []*github.Issue) (runStats, error) {
	for _, i := range issues {
		fmt.Println(fmt.Sprintf("processing issue %s", i.GetHTMLURL()))
		if i.IsPullRequest() {
			run.stats.PullRequest++
			log.Printf(fmt.Sprintf("skip %s: is pull request", i.GetHTMLURL()))
			continue
		}
	
		var commentTpl string
		commentTplParams := []interface{}{i.GetUser().GetLogin()}
		if !isStale(i) {
			log.Debugf(fmt.Sprintf("%s is active", i.GetHTMLURL()))
			run.stats.Active++
			title := i.GetTitle()
			if runID != "" {
				title = fmt.Sprintf("[test][%s] %s", runID, i.GetTitle())
			}
			url, err := Discourse.PostTopic(title, i.GetBody(), discourseCategoryID)
			if err != nil {
				return err
			}
			commentTpl = activeTpl
			commentTplParams = append(commentTplParams, url)
		} else {
			run.stats.Stale++
			commentTpl = staleTpl
		}
	
		if err := github.PostComment(i, fmt.Sprintf(commentTpl, commentTplParams...)); err != nil {
			return err
		}
	
		if err := github.Close(i); err != nil {
			return err
		}
	
		if err := github.Lock(i); err != nil {
			return err
		}
	
		run.stats.Processed++
		time.Sleep(time.Millisecond + 1000)
	}
	return run.stats, nil
}
