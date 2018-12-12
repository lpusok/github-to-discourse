package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/github"
)

func saveState(f *os.File, i *github.Issue, state int, extra string, logmsg string) {
	line := fmt.Sprintf("%s %d %s\n", i.GetHTMLURL(), state, extra)

	if _, err := f.WriteString(line); err != nil {
		fmt.Printf("save state: %s", err)
		os.Exit(1)
	}

	f.Sync()

	fmt.Println()
	fmt.Println(fmt.Sprintf(logmsg))
}

func isStale(i *github.Issue) bool {
	threeMonthsAgo := time.Now().AddDate(0, -3, 0)
	return i.GetUpdatedAt().Before(threeMonthsAgo)
}

func discourse(tc *http.Client, i *github.Issue, mode string) (string, error) {
	// save to discourse
	discourseUrl := ""

	message := make(map[string]interface{})

	// prepare payload
	if mode == "test" {
		message["title"] = prefixWithRunID(i.GetTitle())
	} else {
		message["title"] = i.GetTitle()
	}
	message["raw"] = i.GetBody()
	message["category"] = buildIssuesCat

	payload, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("could not marshal %s: %s", message, err)
	}

	//////////////////////////
	// post to discourse /////
	//////////////////////////
	key := os.Getenv("DISCOURSE_API_KEY")
	user := os.Getenv("DISCOURSE_USER")
	url := fmt.Sprintf("https://discuss.bitrise.io/posts.json?api_key=%s&api_username=%s", key, user)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return "", fmt.Errorf("error posting payload %s: %s", payload, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read response body: %s", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("api error for payload %s: %s", payload, body)
	}

	//  unmarshal response body
	var data interface{}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	err = decoder.Decode(&data)
	//err = json.Unmarshal(body, &data)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal response body %s: %s", body, err)
	}

	m := data.(map[string]interface{})
	topicID, err := m["topic_id"].(json.Number).Int64()
	if err != nil {
		return "", fmt.Errorf("could not unmarshal response body %s: %s", body, err)
	}
	discourseUrl = fmt.Sprintf("https://discuss.bitrise.io/t/%d", topicID)

	return discourseUrl, nil
}

func comment(tc *http.Client, i *github.Issue, comment string) error {
	// prepare payload
	commentPayload := map[string]interface{}{
		"body": comment,
	}

	commentBytes, err := json.Marshal(commentPayload)
	if err != nil {
		return fmt.Errorf("could not marshal commentPayload %s: %s", commentPayload, err)
	}

	// posting comment to GitHub
	req, err := http.NewRequest("POST", i.GetCommentsURL(), bytes.NewBuffer(commentBytes))
	resp, err := tc.Do(req)
	defer resp.Body.Close()

	if err != nil {
		return fmt.Errorf("error posting payload %s: %s", commentPayload, err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read response body: %s", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("api error for payload %s: %s", commentPayload, body)
	}

	return nil
}

func close(tc *http.Client, i *github.Issue) error {
	payload := map[string]interface{}{
		"state": "closed",
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("could not marshal %s: %s", payload, err)
	}

	request, err := http.NewRequest("PATCH", i.GetURL(), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("could not create request: %s", err)
	}

	resp, err := tc.Do(request)
	defer resp.Body.Close()

	if err != nil {
		return fmt.Errorf("error sending request: %s", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read response body: %s", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("api error for payload %s: %s", payload, body)
	}

	return nil
}

func lock(tc *http.Client, i *github.Issue) error {
	// // lock
	url := fmt.Sprintf("%s/lock", i.GetURL())
	request, err := http.NewRequest("PUT", url, bytes.NewBuffer([]byte{}))
	request.Header.Add("Content-Length", "0")
	if err != nil {
		return fmt.Errorf("could not create request: %s", err)
	}

	resp, err := tc.Do(request)
	defer resp.Body.Close()

	if err != nil {
		return fmt.Errorf("error sending request: %s", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read response body: %s", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("api error: %s", body)
	}

	return nil
}
