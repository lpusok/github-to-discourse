package discourse

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/google/go-github/github"
)

const (
	staffCategory  = 29
	buildIssuesCat = 11
)

var (
	discourseAPIKey     = os.Getenv("DISCOURSE_API_KEY")
	discourseAPIUser    = os.Getenv("DISCOURSE_API_USER")
	discourseCategoryID int
)

func init() {
	if discourseAPIKey == "" {
		fmt.Println("DISCOURSE_API_KEY empty")
		os.Exit(1)
	}

	if discourseAPIUser == "" {
		fmt.Println("DISCOURSE_API_USER empty")
		os.Exit(1)
	}

	flag.IntVar(&discourseCategoryID, "discourse-category-id", staffCategory, "--discourse-category-id=<int> (discourse category to post topics to)")
}

func PostTopic(title string, content string, category int) (string, error) {
	message := make(map[string]interface{})

	message["title"] = title
	message["category"] = category
	message["raw"] = content

	payload, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("could not marshal %s: %s", message, err)
	}

	url := fmt.Sprintf("https://discuss.bitrise.io/posts.json?api_key=%s&api_username=%s", discourseAPIKey, discourseAPIUser)
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