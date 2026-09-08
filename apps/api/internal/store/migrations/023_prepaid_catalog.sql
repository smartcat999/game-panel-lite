CREATE TABLE prepaid_plan_versions (
 plan_id text NOT NULL,
 version bigint NOT NULL CHECK(version > 0),
 terms text NOT NULL,
 published_by text NOT NULL,
 PRIMARY KEY(plan_id,version)
);
CREATE TABLE prepaid_plan_sales (
 plan_id text NOT NULL,
 plan_version bigint NOT NULL CHECK(plan_version > 0),
 enabled boolean NOT NULL DEFAULT false,
 version bigint NOT NULL CHECK(version > 0),
 updated_by text NOT NULL,
 PRIMARY KEY(plan_id,plan_version),
 FOREIGN KEY(plan_id,plan_version) REFERENCES prepaid_plan_versions(plan_id,version)
);
