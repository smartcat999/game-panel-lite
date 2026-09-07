# SSE authorization lifecycle

Server watch and local/remote log connections recheck their session, current account permissions and instance membership at startup and every five seconds. Checks have a two-second database deadline and deny access on lookup errors. Platform administrators use their current platform role; authenticated streams never fall back to pre-setup anonymous access. Anonymous self-hosted streams stop when setup creates an account.

Revocation cancels the request context, closes a blocked local log reader, removes remote subscriptions and expires blocked HTTP writes. Cleanup waits for the write-deadline callback so it cannot run after the connection is reused. A closed stream does not promise a final SSE event. Reconnection passes normal authentication and instance authorization again.

This is bounded periodic revalidation, subject to process scheduling and transport delays, not synchronous revocation or removal of already delivered bytes. File downloads and already accepted background work are outside this mechanism. A production design still needs shared revocation notifications, connection limits, bounded fanout and load testing; per-connection database polling has not been validated for the target SaaS connection volume.

Tests hold real HTTP streams open before revoking membership/session records, covering idle watch, remote logs and a blocked local pipe. Current-role demotion and expired sessions are also checked. No deployed multi-replica or capacity result is implied.
