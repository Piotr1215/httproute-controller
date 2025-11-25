# Security Policy

## Supported Versions

Currently supported versions:

| Version | Supported          |
| ------- | ------------------ |
| 0.3.x   | :white_check_mark: |
| < 0.3   | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly:

### Private Disclosure

For sensitive security issues, please use GitHub's [private vulnerability reporting](https://github.com/Piotr1215/httproute-controller/security/advisories/new).

### Public Issues

For non-sensitive issues, you can [open a public issue](https://github.com/Piotr1215/httproute-controller/issues/new).

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Response Time

- Initial response: within 48 hours
- Status updates: every 7 days
- Fix timeline: depends on severity

## Security Features

- **Signed Container Images**: All images signed with Cosign (keyless)
- **SBOM**: Software Bill of Materials embedded in images
- **Provenance**: Build provenance attestations available
- **Vulnerability Scanning**: Trivy scans on every release
- **Static Analysis**: CodeQL and Gosec in CI/CD
- **Dependency Updates**: Automated with Dependabot

## Verification

Verify image signatures:
```sh
cosign verify \
  --certificate-identity-regexp="https://github.com/Piotr1215/httproute-controller/.*" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  piotrzan/httproute-controller:latest
```
