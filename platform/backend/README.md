# Backend

This directory will contain the new Go platform implementation with three deployable binaries:

- `control-plane`
- `region-controller`
- `node-agent`

Each bounded context owns its schema and exposes a small module interface. Cross-context access uses module interfaces or versioned events, never another module's repository. Production SQL must not use JOIN; compose bounded batch results in Go or maintain an asynchronous read model.

The legacy backend is not a dependency. Game Provider and Runtime Provider code can be migrated later through adapters after their new interfaces have contract tests.
