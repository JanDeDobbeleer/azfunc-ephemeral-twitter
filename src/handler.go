package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ChimeraCoder/anaconda"
)

var (
	consumerKey        string
	consumerSecret     string
	accessToken        string
	accessTokenSecret  string
	maxTweetAge        string
	interactionTimeout string
	whitelist          []string
)

// TwitterAPIClient for mocking out Anaconda
type TwitterAPIClient interface {
	GetSelf(v url.Values) (u anaconda.User, err error)
	GetUserTimeline(v url.Values) (timeline []anaconda.Tweet, err error)
	GetSearch(queryString string, v url.Values) (sr anaconda.SearchResponse, err error)
	DeleteTweet(id int64, trimUser bool) (tweet anaconda.Tweet, err error)
}

func setVariables() {
	consumerKey = getenv("TWITTER_CONSUMER_KEY")
	consumerSecret = getenv("TWITTER_CONSUMER_SECRET")
	accessToken = getenv("TWITTER_ACCESS_TOKEN")
	accessTokenSecret = getenv("TWITTER_ACCESS_TOKEN_SECRET")
	maxTweetAge = getenv("MAX_TWEET_AGE")
	interactionTimeout = getenv("TWEET_INTERACTION_TIMEOUT")
	whitelist = getWhitelist(os.Getenv("WHITELIST"))
}

func getenv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		panic("Missing required environment variable " + name)
	}
	return v
}

func getWhitelist(whiteList string) []string {
	if whiteList == "" {
		return make([]string, 0)
	}
	return strings.Split(whiteList, ":")
}

func getTimeline(api TwitterAPIClient) ([]anaconda.Tweet, error) {
	args := url.Values{}
	args.Add("count", "200")
	args.Add("include_rts", "true")
	timeline, err := api.GetUserTimeline(args)
	if err != nil {
		return make([]anaconda.Tweet, 0), err
	}
	return timeline, nil
}

func getRepliesForTweet(api TwitterAPIClient, tweetID int64) []anaconda.Tweet {
	args := url.Values{}
	args.Add("count", "200")
	args.Add("since_id", strconv.FormatInt(tweetID, 10))
	me, err := api.GetSelf(nil)
	if err != nil {
		return make([]anaconda.Tweet, 0)
	}
	queryString := fmt.Sprintf("to:%s", me.ScreenName)
	searchResponse, err := api.GetSearch(queryString, args)
	if err != nil {
		return make([]anaconda.Tweet, 0)
	}
	replies := searchResponse.Statuses[:0]
	for i := range searchResponse.Statuses {
		if searchResponse.Statuses[i].InReplyToStatusID == tweetID {
			replies = append(replies, searchResponse.Statuses[i])
		}
	}
	return replies
}

func isWhitelisted(id int64) bool {
	tweetID := strconv.FormatInt(id, 10)
	for _, w := range whitelist {
		if w == tweetID {
			log.Print("TWEET IS WHITELISTED: ", tweetID)
			return true
		}
	}
	return false
}

func hasOngoingInteractions(api TwitterAPIClient, tweetID int64, interactionAgeLimit time.Duration) bool {
	replies := getRepliesForTweet(api, tweetID)
	for i := range replies {
		createdTime, err := replies[i].CreatedAtTime()
		if err != nil {
			log.Print("Could not parse time ", err)
			continue
		}
		if time.Since(createdTime) < interactionAgeLimit {
			log.Print("TWEET HAS ONGOING INTERACTIONS: ", tweetID)
			return true
		}
	}
	return false
}

func deleteFromTimeline(api TwitterAPIClient, tweetAgeLimit, interactionAgeLimit time.Duration) {
	log.Print("Start deleting tweets")
	timeline, err := getTimeline(api)
	if err != nil {
		log.Print("Could not get timeline ", err)
	}
	for i := range timeline {
		tweet := timeline[i]
		createdTime, err := tweet.CreatedAtTime()
		if err != nil {
			log.Print("Could not parse time ", err)
			continue
		}
		if time.Since(createdTime) > tweetAgeLimit && !isWhitelisted(tweet.Id) && !hasOngoingInteractions(api, tweet.Id, interactionAgeLimit) {
			_, err := api.DeleteTweet(tweet.Id, true)
			log.Print("DELETED TWEET WITH ID ", tweet.Id)
			log.Print("TWEET ", createdTime, " - ", tweet.Text)
			if err != nil {
				log.Print("Failed to delete: ", err)
			}
		}
	}
	log.Print("No more tweets to delete")
}

func ephemeral(w http.ResponseWriter, r *http.Request) {
	anaconda.SetConsumerKey(consumerKey)
	anaconda.SetConsumerSecret(consumerSecret)
	api := anaconda.NewTwitterApi(accessToken, accessTokenSecret)
	api.SetLogger(anaconda.BasicLogger)

	tweetAgeLimit, _ := time.ParseDuration(maxTweetAge)
	interactionAgeLimit, _ := time.ParseDuration(interactionTimeout)

	deleteFromTimeline(api, tweetAgeLimit, interactionAgeLimit)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Go request received at: %s", r.RequestURI)
}

func main() {
	setVariables()
	listenAddr := ":8080"
	if val, ok := os.LookupEnv("FUNCTIONS_CUSTOMHANDLER_PORT"); ok {
		listenAddr = ":" + val
	}
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/ephemeralTwitter", ephemeral)
	log.Printf("About to listen on %s. Go to https://127.0.0.1%s/", listenAddr, listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
