package runmode

import (
	"flag"
	"fmt"


	"time"

	gh "github.com/google/go-github/github"
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

func DryRun(issues []*gh.Issue) (Stats, error) {
	var stats Stats
	for _, i := range issues {
		log.Printf("process issue %s", i.GetHTMLURL())
		if i.IsPullRequest() {
			stats.PullRequest++
			fmt.Println(fmt.Sprintf("skip %s: is pull request", i.GetHTMLURL()))
			continue
		}
	
		if !github.IsStale(i) {
			stats.Active++
			fmt.Println(fmt.Sprintf("%s is active", i.GetHTMLURL()))
		} else {
			stats.Stale++
			fmt.Println(fmt.Sprintf("%s is stale", i.GetHTMLURL()))
		}
		time.Sleep(time.Millisecond + 1000)
	}
	stats.Processed = len(issues)
	return stats, nil
}

func LiveRun(issues []*gh.Issue) (Stats, error) {
	var stats Stats
	for _, i := range issues {
		log.Printf("process issue %s", i.GetHTMLURL())
		if i.IsPullRequest() {
			stats.PullRequest++
			log.Printf("skip %s: is pull request", i.GetHTMLURL())
			continue
		}
	
		var commentTpl string
		commentTplParams := []interface{}{i.GetUser().GetLogin()}
		if !github.IsStale(i) {
			stats.Active++
			title := i.GetTitle()
			if runID != "" {
				title = fmt.Sprintf("[test][%s] %s", runID, i.GetTitle())
			}
			
			log.Printf("post to discourse")
			url, err := discourse.PostTopic(title, i.GetBody())
			if err != nil {
				return stats, err
			}

			commentTpl = activeTpl
			commentTplParams = append(commentTplParams, url)
		} else {
			log.Printf("skip %s: is stale", i.GetHTMLURL())
			stats.Stale++
			commentTpl = staleTpl
		}
	
		log.Printf("post comment")
		if err := github.PostComment(i, fmt.Sprintf(commentTpl, commentTplParams...)); err != nil {
			return stats, err
		}
		
		log.Printf("close issue")
		if err := github.Close(i); err != nil {
			return stats, err
		}
		
		log.Printf("lock issue")
		if err := github.Lock(i); err != nil {
			return stats, err
		}
	
		stats.Processed++
		time.Sleep(time.Millisecond + 1000)
	}
	return stats, nil
}
