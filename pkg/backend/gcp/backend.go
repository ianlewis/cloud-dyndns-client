// Copyright 2017 Google LLC
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

package gcp

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2/google"
	dns "google.golang.org/api/dns/v1"

	"github.com/ianlewis/cloud-dyndns-client/pkg/backend"
)

var cloudDnsScopes = []string{
	dns.CloudPlatformScope,
	dns.CloudPlatformReadOnlyScope,
	dns.NdevClouddnsReadonlyScope,
	dns.NdevClouddnsReadwriteScope,
}

type cloudDNSRecord struct {
	dns.ResourceRecordSet
}

func (c *cloudDNSRecord) Name() string {
	return c.ResourceRecordSet.Name
}

func (c *cloudDNSRecord) Type() string {
	return c.ResourceRecordSet.Type
}

func (c *cloudDNSRecord) Ttl() int64 {
	return c.ResourceRecordSet.Ttl
}

func (c *cloudDNSRecord) Data() []string {
	return c.ResourceRecordSet.Rrdatas
}

// TODO: New backend

type cloudDNSBackend struct {
	client  *dns.Service
	project string
	zone    string
	timeout time.Duration
}

func NewCloudDNSBackend(project, zone string) (backend.DNSBackend, error) {
	client, err := getDNSClient()
	if err != nil {
		return nil, err
	}

	return &cloudDNSBackend{
		client:  client,
		project: project,
		zone:    zone,
		timeout: 5 * time.Second,
	}, nil
}

func (b *cloudDNSBackend) GetRecord(ctx context.Context, dnsName, dnsType string) (backend.DNSRecord, error) {
	var record *dns.ResourceRecordSet

	call := b.client.ResourceRecordSets.List(b.project, b.zone)

	err := call.Pages(ctx, func(page *dns.ResourceRecordSetsListResponse) error {
		for _, v := range page.Rrsets {
			if v.Name == dnsName && v.Type == dnsType {
				record = v
			}
		}
		return nil // NOTE: returning a non-nil error stops pagination.
	})
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	return &cloudDNSRecord{*record}, nil
}

func (b *cloudDNSBackend) UpdateRecords(ctx context.Context, additions []backend.DNSRecord, deletions []backend.DNSRecord) error {
	a := []*dns.ResourceRecordSet{}
	d := []*dns.ResourceRecordSet{}

	for _, r := range additions {
		a = append(a, &dns.ResourceRecordSet{
			Name:    r.Name(),
			Type:    r.Type(),
			Ttl:     r.Ttl(),
			Rrdatas: r.Data(),
		})
	}

	for _, r := range deletions {
		d = append(d, &dns.ResourceRecordSet{
			Name:    r.Name(),
			Type:    r.Type(),
			Ttl:     r.Ttl(),
			Rrdatas: r.Data(),
		})
	}

	change := &dns.Change{
		Additions: a,
		Deletions: d,
	}

	_, err := b.client.Changes.Create(b.project, b.zone, change).Context(ctx).Do()
	if err != nil {
		return err
	}

	return nil
}

func getDNSClient() (*dns.Service, error) {
	client, err := google.DefaultClient(context.Background(), cloudDnsScopes...)
	if err != nil {
		return nil, fmt.Errorf("Could not create Google Cloud DNS client: %v", err)
	}

	service, err := dns.New(client)
	if err != nil {
		return nil, fmt.Errorf("Could not create Google Cloud DNS client: %v", err)
	}

	return service, nil
}
