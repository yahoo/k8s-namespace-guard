# k8s-namespace-guard

k8s-namespace-guard provides an admission control policy that safeguards against accidental deletion of cluster namespaces.

## Implementation

This is implemented as an [External Admission Webhook](https://kubernetes.io/docs/admin/extensible-admission-controllers/#external-admission-webhooks) with the k8s-namespace-guard service running as a deployment on each cluster.  

The webhook is configured to send admission review requests for *DELETE* operations on `namespace` resources to the k8s-namespace-guard service. 
The k8s-namespace-guard service listens on a HTTPS port and on receiving such requests, it lists the workload resources defined under that namespace.
The DELETE operation is allowed to proceed only when the namespace does NOT contain such workload resources.

The following resources are currently checked for existence:
- pods
- services
- replicasets
- deployments
- statefulsets
- daemonsets
- ingresses
- horizontalpodautoscalers

The k8s-namespace-guard policy implementation enforces that the above listed resources under the namespace should be deleted before it can be removed.   

## Basic Dev Setup

1. Git clone to your local directory.
2. Build binary:
    - Mac os: `go build -i -o k8s-namespace-guard`
    - Rhel: `env GOOS=linux GOARCH=amd64 go build -i -o k8s-namespace-guard`
3. Run binary: `./k8s-namespace-guard`.
4. Follow standard Go code format: `gofmt -w *.go`

## Command Line Args

```
USAGE:
  --admitAll     bool    True to admit all namespace deletions without validation. (default false)
  --certFile     string  The cert file for the https server. (default "/var/lib/kubernetes/kubernetes.pem")
  --clientAuth   bool    True to verify client cert/auth during TLS handshake. (default false)
  --clientCAFile string  The cluster root CA that signs the apiserver cert (default "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
  --keyFile      string  The key file for the https server. (default "/var/lib/kubernetes/kubernetes-key.pem")
  --logFile      string  Log file name and full path. (default "/var/log/nslifecycle.log")
  --logLevel     string  The log level. (default "info")
  --port         string  Server port. (default "443")
```

Copyright 2017 Yahoo Holdings Inc. Licensed under the terms of the 3-Clause BSD License.
