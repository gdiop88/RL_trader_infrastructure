
-- Migration 000001 rollback: destructive; deletes all infrastructure data.

DROP TABLE IF EXISTS infrastructure_audit_log;
DROP TABLE IF EXISTS infrastructure_reconciliations;
DROP TABLE IF EXISTS resource_usage_samples;
DROP TABLE IF EXISTS resource_reservations;
DROP TABLE IF EXISTS volume_attachments;
DROP TABLE IF EXISTS storage_volumes;
DROP TABLE IF EXISTS instance_network_attachments;
DROP TABLE IF EXISTS network_resources;
DROP TABLE IF EXISTS agent_instance_assignments;
DROP TABLE IF EXISTS agent_registry;
DROP TABLE IF EXISTS operation_attempts;
DROP TABLE IF EXISTS operation_events;
DROP TABLE IF EXISTS provisioning_operations;
DROP TABLE IF EXISTS provisioning_templates;
DROP TABLE IF EXISTS infrastructure_instances;
DROP TABLE IF EXISTS infrastructure_projects;
DROP TABLE IF EXISTS compute_profiles;
DROP TABLE IF EXISTS provider_regions;
DROP TABLE IF EXISTS provider_accounts;
DROP TABLE IF EXISTS cloud_providers;
