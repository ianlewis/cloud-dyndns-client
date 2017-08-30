# Cloud Dynamic DNS

This project contains a simple [Dynamic DNS](https://en.wikipedia.org/wiki/Dynamic_DNS) client that can be used with cloud services. It simply gets your current IP address and sets it to DNS records in backing DNS services. It will do it's best to make sure that the DNS record is always there and set to the desired value, even if something or someone updates or deletes it. It is intended to be used where public internet IPs are assigned dynamically, such as home networks.

Currently cloud-dyndns-client only supports Google Cloud Platform. It is planned to add other DNS APIs as backends.

## Prerequisites

cloud-dyndns-client requires **Go 1.8**.

## Install

> You can install Go by following [these instructions](https://golang.org/doc/install).

`cloud-dyndns-client` is written in Go, so if you have Go installed you can install it with
`go get`:

```
go get github.com/IanLewis/cloud-dyndns-client/cmd/cloud-dyndns-client
```

This will download the code, compile it, and leave an `embedmd` binary
in `$GOPATH/bin`.

## Usage

You need to set set up a DNS provider. Currently only Google Cloud Platform is supported.

### Google Cloud Platform

Set up the client to use GCP by first creating a service account.

1. Create a GCP service account

```
SA_EMAIL=$(gcloud iam service-accounts --format='value(email)' create cloud-dyndns-client)
```

2.  Create a JSON key file associated with the new service account

```
gcloud iam service-accounts keys create service-account.json --iam-account=$SA_EMAIL
```

3. Add an IAM policy to the service account for the project.

```
PROJECT=$(gcloud config list core/project --format='value(core.project)')
gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:$SA_EMAIL --role roles/dns.admin
```

### Configuration

Create a `config.json` for the client. Enter the domain name you want to update, the GCP project ID, and Cloud DNS managed zone name. Multiple domains can be added as part of the configuration.

```
{
  "mydomain.example.com": {
    "provider": "gcp",
    "provider_config": {
       "project_id": "example-project",
       "managed_zone": "example-zone",
    }
  }
}
```

### Running the client

Start the app and provide the path to the `config.json`. You need to specify the service account key in the `GOOGLE_APPLICATION_CREDENTIALS` environment variable.

```
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
./cloud-dyndns-client -config config.json
```

### Deploy the client to Kubernetes

1. Create a secret for the json key file

```
kubectl create secret generic cloud-dyndns-client-service-account --from-file=service-account.json
```

2. Deploy the client

```
kubectl apply -f kubernetes/deploy.yaml
```

## Contributing

Please read [CONTRIBUTING.md](CONTRIBUTING.md).

## Disclaimers

This is not an official Google product
