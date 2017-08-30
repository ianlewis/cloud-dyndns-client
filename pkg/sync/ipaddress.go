// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sync

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var webCheck = []string{
	"http://checkip.dyndns.org/",
	"http://ipdetect.dnspark.com/",
	"http://dns.loopia.se/checkip/checkip.php",
}

// IPAddressPoller is a poller used to check the value
// of the current public internet IP address.
type IPAddressPoller struct {
	channels     []chan string
	pollInterval time.Duration
}

func NewIPAddressPoller(pollInterval time.Duration) *IPAddressPoller {
	return &IPAddressPoller{
		pollInterval: pollInterval,
	}
}

// Channel() returns a channel that receives data whenever an
// IP address value is received.
func (p *IPAddressPoller) Channel() <-chan string {
	c := make(chan string, 1)
	p.channels = append(p.channels, c)
	return c
}

// poll() runs a single polling event and retrieving the internet IP.
func (p *IPAddressPoller) poll() error {
	// Shuffle the list of URLs randomly so that they aren't
	// always used in the same order.
	urls := make([]string, len(webCheck))
	copy(urls, webCheck)
	for i := range urls {
		j := rand.Intn(i + 1)
		urls[i], urls[j] = urls[j], urls[i]
	}

	// Make a request to each url and send to the
	// channels if an IP is retrieved
	var lastErr error
	for i := range urls {
		ip, err := request(urls[i])
		if err != nil {
			lastErr = err
			continue
		}

		for _, c := range p.channels {
			select {
			case c <- ip:
			default:
			}
		}
		return nil
	}

	return fmt.Errorf("Could not obtain IP address: %s %#v", lastErr.Error(), lastErr)
}

// request() makes a request to a URL to get the internet IP address.
func request(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Got status code from %s: %s", url, resp.StatusCode)
	}

	z := html.NewTokenizer(resp.Body)
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return "", z.Err()
		case html.TextToken:
			text := strings.Trim(string(z.Text()), " \n\t")
			if text != "" {
				ip := ""
				fmt.Sscanf(text, "Current IP Address: %s", &ip)
				if ip != "" {
					return strings.Trim(ip, " \n\t"), nil
				}
			}
		}
	}

	return "", fmt.Errorf("Could not obtain IP address from html body")
}

// Run() starts the main loop for the poller.
func (i *IPAddressPoller) Run(stopCh <-chan struct{}) error {
	if err := i.poll(); err != nil {
		log.Printf("Error polling for IP: %s %#v", err.Error(), err)
	}
	for {
		select {
		case <-time.After(i.pollInterval):
			if err := i.poll(); err != nil {
				log.Printf("Error polling for IP: %s %#v", err.Error(), err)
			}
		case <-stopCh:
			return nil
		}
	}
	return nil
}
