package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/google/go-github/github"
)

type dryRun struct {
	stats runStats
}

type liveRun struct {
	stats  runStats
	tc     *http.Client
	chkptf *os.File
}

func (run dryRun) process(i *github.Issue) {
	if i.IsPullRequest() {
		run.stats.PullRequest++
		fmt.Println(fmt.Sprintf("skip %s: is pull request", i.GetHTMLURL()))
	}

	if !isStale(i) {
		run.stats.Active++
		fmt.Println(fmt.Sprintf("%s is active", i.GetHTMLURL()))
	} else {
		run.stats.Stale++
		fmt.Println(fmt.Sprintf("%s is stale", i.GetHTMLURL()))
	}
	run.stats.Processed++
}

func (run *liveRun) process(i *github.Issue, mode string) error {
	if i.IsPullRequest() {
		run.stats.PullRequest++
		fmt.Println(fmt.Sprintf("skip %s: is pull request", i.GetHTMLURL()))
		return nil
	}

	if !isStale(i) {

		fmt.Println(fmt.Sprintf("%s is active", i.GetHTMLURL()))
		run.stats.Active++

		// discourse
		url, err := discourse(i, mode)
		if err != nil {
			return err
		}

		if err := saveState(run.chkptf, i, discourseDone, url, fmt.Sprintf(discourseLog, url)); err != nil {
			return fmt.Errorf("process: %s", err)
		}

		// comment
		if err := comment(i, fmt.Sprintf(activeTpl, i.GetUser().GetLogin(), url)); err != nil {
			return err
		}
	} else {

		run.stats.Stale++
		printIssueLog("Issue is stale")
		fmt.Println()

		// comment
		if err := comment(i, fmt.Sprintf(staleTpl, i.GetUser().GetLogin())); err != nil {
			return err
		}
	}

	if err := saveState(run.chkptf, i, commentDone, "", commentLog); err != nil {
		return fmt.Errorf("process: %s", err)
	}

	// close
	if err := close(i); err != nil {
		return err
	}

	if err := saveState(run.chkptf, i, closeDone, "", closeLog); err != nil {
		return fmt.Errorf("process: %s", err)
	}

	// lock
	if err := lock(i); err != nil {
		return err
	}

	if err := saveState(run.chkptf, i, lockDone, "", lockLog); err != nil {
		return fmt.Errorf("process: %s", err)
	}
	run.stats.Processed++

	return nil
}
