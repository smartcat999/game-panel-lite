CREATE TABLE workload_artifact_references (
    assignment_id text NOT NULL REFERENCES workload_assignments(id) ON DELETE CASCADE,
    artifact_id text NOT NULL,
    organization_id text NOT NULL,
    PRIMARY KEY (assignment_id, artifact_id)
);
CREATE INDEX idx_workload_artifact_references_owner_source
    ON workload_artifact_references (organization_id, artifact_id);

-- Preserve references from every persisted manifest, even older generations.
-- Source ownership survives instance deletion or reassignment.
INSERT INTO workload_artifact_references (assignment_id, artifact_id, organization_id)
SELECT DISTINCT a.id, artifact->>'id', m.organization_id
FROM workload_assignments a
CROSS JOIN LATERAL jsonb_array_elements(COALESCE(NULLIF(NULLIF(a.spec, '')::jsonb->'options'->'artifacts', 'null'::jsonb), '[]'::jsonb)) artifact
JOIN mod_files m ON m.id = artifact->>'id'
WHERE m.organization_id <> '';
