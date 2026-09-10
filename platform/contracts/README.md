# Contracts

This directory owns versioned interfaces shared across processes:

- public and operator OpenAPI documents
- Control Plane to Region event envelopes
- Region to Control Plane observation events
- Region to Node assignment contracts
- stable identifiers, versions, idempotency keys, and error codes

Contracts contain no database models and no provider implementation types. Breaking message changes require a new version and a documented compatibility window.

V1 contract rules:

- Public endpoints only expose system-allocated bindings; protocols and listener requirements come from an immutable Provider Manifest.
- Mutations that require external I/O return a durable Operation with HTTP 202.
- Provider configuration uses a controlled, versioned schema plus UI metadata. Unknown field types fail closed.
- Authorization uses typed Action values and built-in Role Bindings. A batch is capped at 100 resources and evaluated after one binding load.
- CNY Wallet and Ledger records replace plans, orders, payments, and entitlements.
- Logs, metrics, usage, and backups belong to a stable Logical Instance ID. Runtime identifiers are provenance only.
