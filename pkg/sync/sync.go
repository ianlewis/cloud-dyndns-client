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

// This package handles syncing with Cloud DNS records.

package sync

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ianlewis/cloud-dyndns-client/pkg/backend"
)

type Record struct {
	Record  backend.DNSRecord
	Backend backend.DNSBackend
}

// domainObj represents a managed DNS record.
type domainObj struct {
	// remote is a local cache of the DNS record
	// as present in the backend provider
	remote backend.DNSRecord
	// managed is a local cache of the desired state
	// of the DNS record
	managed backend.DNSRecord

	// backend is the provider backend for updating
	// DNS records.
	backend backend.DNSBackend

	// Lock per domain
	lock sync.Mutex

	// needSync is a channel that receives data when
	// a sync is required.
	needSync chan struct{}
	// initialized begins as false and becomes true
	// when remote cache data is initialized.
	initialized bool
}

type Syncer struct {
	domains []*domainObj

	// pollInterval is the interval between polling the backend
	// provider and updating the remote cache.
	pollInterval time.Duration
	// syncReconcileInterval is the interval between
	// reconciliation sync checks.
	syncReconcileInterval time.Duration
	// apiTimeout is used for setting timeouts on requests to backends.
	apiTimeout time.Duration
}

func NewSyncer(records []Record, pollInterval, apiTimeout time.Duration) *Syncer {
	// Convert to domainObj
	o := []*domainObj{}
	for _, r := range records {
		o = append(o, &domainObj{
			remote:      r.Record,
			managed:     r.Record,
			backend:     r.Backend,
			needSync:    make(chan struct{}, 1),
			initialized: false,
		})
	}

	return &Syncer{
		domains:               o,
		pollInterval:          pollInterval,
		syncReconcileInterval: 1 * time.Minute,
		apiTimeout:            apiTimeout,
	}
}

// UpdateRecord() updates a managed record so that it can be synced by the
// sync loop.
func (s *Syncer) UpdateRecord(dnsName, dnsType string, ttl int64, data []string) error {
	for _, d := range s.domains {
		if d.managed.Name() == dnsName && d.managed.Type() == dnsType {
			d.lock.Lock()
			d.managed = backend.NewDNSRecord(
				dnsName,
				dnsType,
				ttl,
				data,
			)
			d.lock.Unlock()

			d.needSync <- struct{}{}

			return nil
		}
	}

	return fmt.Errorf("Domain %s not registered.", dnsName)
}

// needsUpdate() compares the left/local DNSRecord against the right/remote
// DNSRecord and returns true if the record needs to be updated in the remote
// backend.
func needsUpdate(l, r backend.DNSRecord) bool {
	if l == nil {
		return false
	}
	if r == nil {
		return true
	}

	if l.Name() != r.Name() || l.Type() != r.Type() {
		// Incomparable
		return false
	}

	if l.Ttl() != r.Ttl() {
		return true
	}

	lData := l.Data()
	rData := r.Data()

	if len(lData) <= 0 {
		return false
	}

	if len(lData) != len(rData) {
		return true
	}

	for i, d := range lData {
		if d != rData[i] {
			return true
		}
	}

	return false
}

// pollSingle() performs a single refresh of the remote cache value.
func (s *Syncer) pollSingle(d *domainObj) error {
	// Update record sets to reconcile
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, s.apiTimeout)
	defer cancel()

	record, err := d.backend.GetRecord(ctx, d.managed.Name(), d.managed.Type())
	if err != nil {
		return err
	}

	d.lock.Lock()
	d.remote = record
	d.initialized = true
	d.needSync <- struct{}{}
	d.lock.Unlock()

	return nil
}

// poll() runs a loop to poll the backend and update the remote cache.
func (s *Syncer) poll(d *domainObj, stopCh <-chan struct{}) {
	// Start by polling the domain to initialize the remote value and local cache
	// TODO: Implement some exponential retry backoff logic in case poll interval is long
	if err := s.pollSingle(d); err != nil {
		log.Printf("Error polling DNS record %q: %v", d.managed.Name(), err)
	}
	for {
		select {
		case <-time.After(s.pollInterval):
			if err := s.pollSingle(d); err != nil {
				log.Printf("Error polling DNS record %q: %v", d.managed.Name(), err)
			}
		case <-stopCh:
			return
		}
	}
}

// syncSingle() syncs a single record entry, updating it with
// the backend server if necessary.
func (s *Syncer) syncSingle(d *domainObj) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	// Sanity check
	if !needsUpdate(d.managed, d.remote) {
		return nil
	}

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, s.apiTimeout)
	defer cancel()

	additions := []backend.DNSRecord{d.managed}
	deletions := []backend.DNSRecord{}
	if d.remote != nil {
		deletions = []backend.DNSRecord{d.remote}
	}
	log.Printf("Updating record %v: old: %v, new: %v", d.managed.Name(), d.remote.Data()[0], d.managed.Data()[0])
	err := d.backend.UpdateRecords(ctx, additions, deletions)
	if err != nil {
		return err
	}

	d.remote = d.managed

	return nil
}

// sync() runs a loop to sync records with backend providers
// when necessary.
func (s *Syncer) sync(d *domainObj, stopCh <-chan struct{}) {
	for {
		select {
		case <-d.needSync:
			if d.initialized {
				if err := s.syncSingle(d); err != nil {
					log.Printf("Error syncing DNS record %s %#v %#v", err.Error(), d, err)
				}
			}
		case <-time.After(s.syncReconcileInterval):
			if d.initialized {
				if err := s.syncSingle(d); err != nil {
					log.Printf("Error syncing DNS record %s %#v %#v", err.Error(), d, err)
				}
			}
		case <-stopCh:
			return
		}
	}
}

// Run() starts the sync and poll loops
func (s *Syncer) Run(stopCh <-chan struct{}) error {
	// Run a sync and poll loop for each domain. Sync and poll loops are
	// separated so that polling and syncing do not block on each other.
	for _, d := range s.domains {
		go s.poll(d, stopCh)
		go s.sync(d, stopCh)
	}

	<-stopCh

	return nil
}
