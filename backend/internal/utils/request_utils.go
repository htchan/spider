package utils

import (
	"strconv"
	"time"

	"io/ioutil"
	// "math/rand"
	"net/http"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

var SlowRequest = false

func getWeb(url string) string {
	c := &http.Client{ Timeout: 5 * time.Second }
	resp, err := c.Get(url)
	if err != nil {
		return ""
	}
	if resp.StatusCode >= 300 {
		return strconv.Itoa(resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	resp.Body.Close()
	c.CloseIdleConnections()
	return string(body)
}

// const MIN_SLEEP_MS_MULTIPLIER = 2000
// const MAX_SLEEP_MS = 30000

func GetWeb(url string, trial int, decoder *encoding.Decoder, constSleep int) (html string, i int) {
	for i = 0; true; i++ {
		html = getWeb(url)
		if statusCode, err := strconv.Atoi(html); err == nil || (len(html) == 0) {
			if statusCode == 503 {
				if i >= 100 { return }
				time.Sleep(time.Duration(i) * time.Second)
				go func() {
					SlowRequest = true
					time.Sleep(10 * time.Second)
					SlowRequest = false
				}()
			} else {
				if i >= 10 { return }
				time.Sleep(time.Duration((i + 1) * constSleep) * time.Millisecond)
			}
			continue
		}
		if (len(html) == 0) {
			if i >= 10 { return }
			time.Sleep(time.Duration((i + 1) * constSleep) * time.Millisecond)
			continue
		}
		if decoder != nil {
			html, _, _ = transform.String(decoder, html)
		}
		break
	}
	return
}

func RequestInterval() {
	if SlowRequest {
		time.Sleep(1 * time.Second)
	}
}
