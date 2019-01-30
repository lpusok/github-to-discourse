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
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()

	var data map[string]interface{}
	if err := decoder.Decode(&data); err != nil {
		return "", fmt.Errorf("could not unmarshal response body %s: %s", body, err)
	}

	topicID, err := data["topic_id"].(json.Number).Int64()
	if err != nil {
		return "", fmt.Errorf("could not unmarshal response body %s: %s", body, err)
	}
	discourseURL := fmt.Sprintf("https://discuss.bitrise.io/t/%d", topicID)

	return discourseURL, nil
}
