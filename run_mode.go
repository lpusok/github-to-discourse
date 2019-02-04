package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

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

func (run dryRun) process(tc *http.Client, i *github.Issue, f *os.File, mode string) error {

	// skip if PR
	if i.IsPullRequest() {
		run.stats.PullRequest++
		fmt.Println(fmt.Sprintf("skip %s: is pull request", i.GetHTMLURL()))
		return nil
	}

	if !isStale(i) {
		run.stats.Active++
		fmt.Println(fmt.Sprintf("%s is active", i.GetHTMLURL()))
	} else {
		run.stats.Stale++
		fmt.Println(fmt.Sprintf("%s is stale", i.GetHTMLURL()))
	}
	run.stats.Processed++

	return nil
}

func (run *liveRun) process(i *github.Issue, mode string) error {

	// avoid throttling
	time.Sleep(time.Millisecond + 1000)

	// skip if PR
	if i.IsPullRequest() {
		run.stats.PullRequest++
		printSkipPR(i.GetNumber(), i.GetHTMLURL())
		fmt.Println()
		return nil
	}

	// short circuit if reached processing limit
	if run.stats.Processed == maxCount {
		printMaxCountReached()
		fmt.Println()
		return nil
	}

	fmt.Println()

	if !isStale(i) {

		printIssueLog("Issue is active")
		fmt.Println()
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
