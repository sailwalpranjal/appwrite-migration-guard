# Security Policy

Appwrite Migration Guard (amg) is a local CLI that reads from Appwrite
projects using credentials you supply. It does not run as a hosted
service and does not transmit your data anywhere except to the Appwrite
endpoints you configure.

## Reporting a vulnerability

Please open a private security advisory on GitHub
(`Security` tab → `Report a vulnerability`) rather than a public issue.
Include:

- amg version (`amg version`)
- Appwrite version/target (Cloud or self-hosted version)
- Steps to reproduce
- Impact you believe it has

## Handling of credentials

- API keys are read only from environment variables or a local `.env`
  file that you control.
- Keys are sent only as the `X-Appwrite-Key` HTTP header on requests you
  initiate; they are never written to manifests, JSON reports, HTML
  reports, or log output.
- amg does not phone home, does not send telemetry, and does not require
  any account or hosted backend of its own.

## Scope

In scope: amg's own code (`cmd/`, `internal/`). Out of scope: Appwrite
itself — please report Appwrite server/SDK vulnerabilities to the
[Appwrite project](https://github.com/appwrite/appwrite/security).
