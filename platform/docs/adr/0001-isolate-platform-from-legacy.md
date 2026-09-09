# Isolate the hosted platform from the legacy panel

The hosted product is implemented under `platform/` without imports from the legacy frontend or backend because their single-panel page structure and ownership model would silently define the new system. Legacy code may contribute only Game Provider and Runtime Provider implementations, migrated through new contract-tested adapters after the platform seams are stable.
