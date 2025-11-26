# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.7] - 2025-11-26

### Added
- **ReferenceGrant opt-out**: New `httproute.controller/skip-reference-grant` annotation allows users to skip automatic ReferenceGrant creation for manual management
- **Kubernetes Events**: Controller now emits events on Services for all HTTPRoute and ReferenceGrant operations
  - `HTTPRouteReconciled` / `HTTPRouteFailed` / `HTTPRouteDeleted`
  - `ReferenceGrantReconciled` / `ReferenceGrantFailed` / `ReferenceGrantDeleted` / `ReferenceGrantSkipped`
- **Section name override**: New `httproute.controller/section-name` annotation to override gateway listener section per-service
- **E2E tests**: Added HTTPRoute-specific e2e tests for automated validation

### Changed
- **BREAKING**: Annotation prefix is now fixed to `httproute.controller` (no longer configurable)
- **BREAKING**: Gateway name and namespace are now **required** flags (no defaults)
- Improved logging for ReferenceGrant operations with security context

### Fixed
- Configuration mismatch issues from v0.3.6 where different annotation prefixes caused reconciliation loops

## [0.3.6] - 2025-11-26

### Added
- Configurable annotation prefix via `--annotation-prefix` flag
- Configurable gateway defaults via `--default-gateway` and `--default-gateway-namespace` flags
- Configurable section name via `--default-section-name` flag

### Changed
- Default annotation prefix changed from `gateway.homelab.local` to `httproute.controller`

## [0.3.5] - 2025-11-25

### Fixed
- Various stability improvements
- CI/CD pipeline updates

## [0.3.0] - 2025-11-24

### Added
- Initial open source release
- HTTPRoute auto-generation from Service annotations
- ReferenceGrant auto-creation for cross-namespace routing
- Finalizer-based cleanup for cross-namespace HTTPRoutes
- OwnerReference-based cleanup for same-namespace ReferenceGrants
- Helm chart for easy installation
- Multi-platform Docker images (amd64, arm64)
- Cosign image signing and SLSA provenance attestations

[Unreleased]: https://github.com/Piotr1215/httproute-controller/compare/v0.3.7...HEAD
[0.3.7]: https://github.com/Piotr1215/httproute-controller/compare/v0.3.6...v0.3.7
[0.3.6]: https://github.com/Piotr1215/httproute-controller/compare/v0.3.5...v0.3.6
[0.3.5]: https://github.com/Piotr1215/httproute-controller/compare/v0.3.0...v0.3.5
[0.3.0]: https://github.com/Piotr1215/httproute-controller/releases/tag/v0.3.0
