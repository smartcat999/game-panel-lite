ALTER TABLE regions
    ADD COLUMN localized_names jsonb NOT NULL DEFAULT '{}'::jsonb;

UPDATE regions
SET localized_names = '{"zh-CN":"亚洲东部","en":"Asia East"}'::jsonb
WHERE code = 'asia-east';
