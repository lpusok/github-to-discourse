package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-github/github"
)

func discourse(tc *http.Client, i *github.Issue, mode string) (string, error) {
	message := make(map[string]interface{})

	// prepare payload
	if mode == "test" {
		message["title"] = prefixWithRunID(i.GetTitle())
		message["category"] = staffCategory
	} else {
		message["title"] = i.GetTitle()
		message["category"] = buildIssuesCat
	}
	message["raw"] = i.GetBody()

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

	body, err := ioutil.ReadAll(resp.Body)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("warning: could not close response body: %s", err)
		}
	}()
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

	m, ok := data.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("could not convert to map %s", err)
	}
	topicID, err := m["topic_id"].(json.Number).Int64()
	if err != nil {
		return "", fmt.Errorf("could not unmarshal response body %s: %s", body, err)
	}
	discourseURL := fmt.Sprintf("https://discuss.bitrise.io/t/%d", topicID)

	return discourseURL, nil
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
	if err != nil {
		return fmt.Errorf("could not create request: %s", err)
	}

	resp, err := tc.Do(req)
	if err != nil {
		return fmt.Errorf("could not send request: %s", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("warning: could not close response body: %s", err)
		}
	}()

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
	if err != nil {
		return fmt.Errorf("could not send request: %s", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("warning: could not close response body: %s", err)
		}
	}()

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
	if err != nil {
		return fmt.Errorf("could not send request: %s", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("warning: could not close response body: %s", err)
		}
	}()

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
