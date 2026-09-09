# Region is the independent execution domain

A Region owns multiple Nodes plus its own controller, database, durable inbox and outbox, scheduler, monitoring, and object-storage policy. We omit a Cell layer because it duplicates the only execution and failure-isolation level currently required; a future internal partition must remain invisible to global and tenant contracts.
