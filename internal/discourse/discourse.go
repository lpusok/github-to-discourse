package discourse

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	internalTestCategory  = 29
	buildIssuesCat = 11
)

var (
	discourseAPIKey     = os.Getenv("DISCOURSE_API_KEY")
	discourseAPIUser    = os.Getenv("DISCOURSE_API_USER")
	discourseCategoryID int
	topicTpl = `Original GitHub post: %s
	
	%s`
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

	flag.IntVar(&discourseCategoryID, "discourse-category-id", internalTestCategory, "--discourse-category-id=<int> (discourse category to post topics to)")
}

func PostTopic(title string, originURL, content string) (string, error) {
	message := map[string]interface{}{
		"title": title,
		"category": discourseCategoryID,
		"raw": fmt.Sprintf(topicTpl, originURL, content),
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("could not marshal %s; reason: %s", message, err)
	}

	queryStr := url.Values{
		"api_key": []string{discourseAPIKey},
		"api_username": []string{discourseAPIUser},
	}.Encode()
	url := fmt.Sprintf("https://discuss.bitrise.io/posts.json?%s", queryStr)
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
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("api error for payload %s; response body: %s", payload, body)
	}

	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()

	var data map[string]interface{}
	if err := decoder.Decode(&data); err != nil {
		return "", fmt.Errorf("could not unmarshal response body %s; reason: %s", body, err)
	}

	topicID, err := data["topic_id"].(json.Number).Int64()
	if err != nil {
		return "", fmt.Errorf("could not unmarshal response body %s; reason: %s", body, err)
	}
	discourseURL := fmt.Sprintf("https://discuss.bitrise.io/t/%d", topicID)

	return discourseURL, nil
}
