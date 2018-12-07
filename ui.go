package main

import (
	"fmt"
	"strings"
)

func printSkipPR(issNum int, url string) {
	printIssueLog(fmt.Sprintf("Issue #%d (%s) is PR. Skipping.", issNum, url))
}

func printMaxCountReached() {
	fmt.Printf("reached maximum processing count of %d", maxCount)
	fmt.Println()
}

func printIssueHeader(issuesN int, totalN int, issNum int, url string) {
	fmt.Println(indentIssueLog(fmt.Sprintf("(%d/%d) Processing issue #%d", issuesN, totalN, issNum)))
	fmt.Println()
	fmt.Println(indentIssueLog(fmt.Sprintf("%s", url)))
	fmt.Println()
	fmt.Println(indentIssueLog(fmt.Sprintf("%s", strings.Repeat("=", len(url)))))
	fmt.Println()
}

func indentIssueLog(s string) string {
	return strings.Repeat(" ", 8) + s
}

func printIssueLog(s string) {
	fmt.Println(indentIssueLog(s))
}
