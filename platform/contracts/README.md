# Contracts

This directory owns versioned interfaces shared across processes:

- public and operator OpenAPI documents
- Control Plane to Region event envelopes
- Region to Control Plane observation events
- Region to Node assignment contracts
- stable identifiers, versions, idempotency keys, and error codes

Contracts contain no database models and no provider implementation types. Breaking message changes require a new version and a documented compatibility window.
