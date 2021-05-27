# Using secrets with kubectl-trace

In order to upload traces to blob storage, kubectl-trace will need a way to
authenticate with a cloud provider. To support this, kubectl-trace offers the
following flag:

```
--google-application-secret # for uploading traces to GCS on Google Cloud Platform
```

With these flags, you can then use a bucket URI pattern for the `--output` flag
of kubectl-trace.

This allows for centralized storage of traces, and easy integration with third
party tools for analyzing trace data, in the way that is most appropriate to
you or your organization.

## Secrets for Google Cloud Storage

To authenticate with Google Cloud Storage, a one-time setup is required where
secrets must be created ahead of time.

First, create a service account with permissions to upload to a bucket where
traces shall be stored.

Next, download the service account key for this service account, and create a
secret in the namespace that traces will be running in:

```
kubectl create secret generic kubectl-trace-gcp-key --from-file=key.json=PATH-TO-KEY-FILE.json
```

**Note**: the `key.json` above is critical to ensure the secret is projected correctly

When you create a trace, use the following flags:

```
--google-application-secret=kubectl-trace-gcp-key
```
