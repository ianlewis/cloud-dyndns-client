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

package backend

import (
	"context"
)

type DNSRecord interface {
	Name() string
	Type() string
	Ttl() int64
	Data() []string
}

type dnsRecord struct {
	name    string
	dnsType string
	ttl     int64
	data    []string
}

func (r *dnsRecord) Name() string {
	return r.name
}

func (r *dnsRecord) Type() string {
	return r.dnsType
}

func (r *dnsRecord) Ttl() int64 {
	return r.ttl
}

func (r *dnsRecord) Data() []string {
	return r.data
}

func NewDNSRecord(dnsName, dnsType string, ttl int64, data []string) DNSRecord {
	return &dnsRecord{
		name:    dnsName,
		dnsType: dnsType,
		ttl:     ttl,
		data:    data,
	}
}

type DNSBackend interface {
	GetRecord(context.Context, string, string) (DNSRecord, error)
	UpdateRecords(context.Context, []DNSRecord, []DNSRecord) error
}
