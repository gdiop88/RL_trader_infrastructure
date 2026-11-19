
-- Migration 000001: initial RL Trader infrastructure schema (20 tables)
-- Apply only to an empty, approved database.

CREATE TABLE cloud_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    provider_type VARCHAR(50) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    api_endpoint VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE provider_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL,
    name VARCHAR(150) NOT NULL,
    external_account_id VARCHAR(255),
    credential_reference VARCHAR(500),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (provider_id) REFERENCES cloud_providers(id)
);

CREATE TABLE provider_regions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL,
    code VARCHAR(100) NOT NULL,
    display_name VARCHAR(150),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    capabilities JSONB,
    FOREIGN KEY (provider_id) REFERENCES cloud_providers(id),
    UNIQUE (provider_id, code)
);

CREATE TABLE compute_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL,
    name VARCHAR(150) NOT NULL,
    provider_size VARCHAR(150) NOT NULL,
    vcpu_count INTEGER,
    memory_mb INTEGER,
    disk_gb INTEGER,
    gpu_count INTEGER DEFAULT 0,
    gpu_model VARCHAR(150),
    architecture VARCHAR(50),
    hourly_price NUMERIC(18,6),
    currency VARCHAR(10),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    FOREIGN KEY (provider_id) REFERENCES cloud_providers(id),
    CONSTRAINT compute_profile_nonnegative CHECK (vcpu_count IS NULL OR vcpu_count >= 0),
    CONSTRAINT compute_memory_nonnegative CHECK (memory_mb IS NULL OR memory_mb >= 0),
    CONSTRAINT compute_disk_nonnegative CHECK (disk_gb IS NULL OR disk_gb >= 0),
    CONSTRAINT compute_gpu_nonnegative CHECK (gpu_count IS NULL OR gpu_count >= 0)
);

CREATE TABLE infrastructure_projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(150) NOT NULL UNIQUE,
    environment VARCHAR(50) NOT NULL,
    description TEXT,
    owner_reference VARCHAR(255),
    budget_limit NUMERIC(18,2),
    currency VARCHAR(10),
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE infrastructure_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(50) NOT NULL,
    provider_instance_id VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    region VARCHAR(100),
    instance_type VARCHAR(100),
    public_ip INET,
    private_ip INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_error TEXT,
    provisioned_at TIMESTAMPTZ,
    terminated_at TIMESTAMPTZ,
    provider_metadata_id VARCHAR(255),
    project_id UUID,
    provider_account_id UUID,
    compute_profile_id UUID,
    desired_state VARCHAR(50),
    last_observed_at TIMESTAMPTZ,
    FOREIGN KEY (project_id) REFERENCES infrastructure_projects(id),
    FOREIGN KEY (provider_account_id) REFERENCES provider_accounts(id),
    FOREIGN KEY (compute_profile_id) REFERENCES compute_profiles(id),
    UNIQUE (provider, provider_instance_id),
    CONSTRAINT instances_status_check CHECK (status IN ('pending','provisioning','running','stopped','failed','deleting','deleted')),
    CONSTRAINT instances_metadata_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT instances_tags_array CHECK (jsonb_typeof(tags) = 'array')
);

CREATE TABLE provisioning_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID,
    name VARCHAR(150) NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    description TEXT,
    provider VARCHAR(50),
    image_reference VARCHAR(255),
    configuration JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES infrastructure_projects(id)
);

CREATE TABLE provisioning_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID,
    operation_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    requested_by VARCHAR(255),
    provider VARCHAR(50) NOT NULL,
    region VARCHAR(100),
    request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_payload JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMPTZ,
    idempotency_key VARCHAR(255) UNIQUE,
    correlation_id UUID,
    timeout_at TIMESTAMPTZ,
    project_id UUID,
    template_id UUID,
    provider_account_id UUID,
    FOREIGN KEY (instance_id) REFERENCES infrastructure_instances(id) ON DELETE SET NULL,
    FOREIGN KEY (project_id) REFERENCES infrastructure_projects(id),
    FOREIGN KEY (template_id) REFERENCES provisioning_templates(id),
    FOREIGN KEY (provider_account_id) REFERENCES provider_accounts(id),
    CONSTRAINT operations_type_check CHECK (operation_type IN ('create','update','delete','start','stop','restart','resize')),
    CONSTRAINT operations_status_check CHECK (status IN ('pending','running','completed','failed','cancelled')),
    CONSTRAINT operations_retry_check CHECK (retry_count >= 0 AND max_retries >= 0),
    CONSTRAINT operations_request_object CHECK (jsonb_typeof(request_payload) = 'object'),
    CONSTRAINT operations_response_object CHECK (response_payload IS NULL OR jsonb_typeof(response_payload) = 'object'),
    CONSTRAINT operations_time_check CHECK (completed_at IS NULL OR started_at IS NULL OR completed_at >= started_at)
);

CREATE TABLE operation_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    status VARCHAR(50),
    message TEXT,
    event_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sequence_number INTEGER,
    FOREIGN KEY (operation_id) REFERENCES provisioning_operations(id) ON DELETE CASCADE,
    UNIQUE (operation_id, sequence_number),
    CONSTRAINT events_data_object CHECK (jsonb_typeof(event_data) = 'object'),
    CONSTRAINT events_sequence_nonnegative CHECK (sequence_number IS NULL OR sequence_number >= 0)
);

CREATE TABLE operation_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id UUID NOT NULL,
    attempt_number INTEGER NOT NULL,
    status VARCHAR(50) NOT NULL,
    worker_reference VARCHAR(255),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    error_code VARCHAR(100),
    error_message TEXT,
    response_metadata JSONB,
    FOREIGN KEY (operation_id) REFERENCES provisioning_operations(id) ON DELETE CASCADE,
    UNIQUE (operation_id, attempt_number),
    CONSTRAINT attempts_number_positive CHECK (attempt_number > 0)
);

CREATE TABLE agent_registry (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_reference VARCHAR(255) NOT NULL UNIQUE,
    agent_type VARCHAR(100),
    owner_reference VARCHAR(255),
    status VARCHAR(50),
    capabilities JSONB,
    registered_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ
);

CREATE TABLE agent_instance_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL,
    instance_id UUID NOT NULL,
    assignment_role VARCHAR(100),
    status VARCHAR(50) NOT NULL,
    assigned_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    metadata JSONB,
    FOREIGN KEY (agent_id) REFERENCES agent_registry(id),
    FOREIGN KEY (instance_id) REFERENCES infrastructure_instances(id)
);

CREATE TABLE network_resources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID,
    provider_account_id UUID,
    provider_network_id VARCHAR(255),
    name VARCHAR(150),
    region VARCHAR(100),
    network_type VARCHAR(50),
    cidr CIDR,
    status VARCHAR(50),
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES infrastructure_projects(id),
    FOREIGN KEY (provider_account_id) REFERENCES provider_accounts(id)
);

CREATE TABLE instance_network_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL,
    network_id UUID NOT NULL,
    private_ip INET,
    public_ip INET,
    interface_name VARCHAR(100),
    is_primary BOOLEAN DEFAULT FALSE,
    attached_at TIMESTAMPTZ,
    detached_at TIMESTAMPTZ,
    FOREIGN KEY (instance_id) REFERENCES infrastructure_instances(id),
    FOREIGN KEY (network_id) REFERENCES network_resources(id)
);

CREATE TABLE storage_volumes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID,
    provider_account_id UUID,
    provider_volume_id VARCHAR(255),
    name VARCHAR(150),
    region VARCHAR(100),
    size_gb INTEGER NOT NULL,
    volume_type VARCHAR(100),
    status VARCHAR(50),
    encrypted BOOLEAN,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    FOREIGN KEY (project_id) REFERENCES infrastructure_projects(id),
    FOREIGN KEY (provider_account_id) REFERENCES provider_accounts(id),
    CONSTRAINT storage_size_positive CHECK (size_gb > 0)
);

CREATE TABLE volume_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    volume_id UUID NOT NULL,
    instance_id UUID NOT NULL,
    mount_point VARCHAR(255),
    status VARCHAR(50),
    attached_at TIMESTAMPTZ,
    detached_at TIMESTAMPTZ,
    FOREIGN KEY (volume_id) REFERENCES storage_volumes(id),
    FOREIGN KEY (instance_id) REFERENCES infrastructure_instances(id)
);

CREATE TABLE resource_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    agent_id UUID,
    instance_id UUID,
    reservation_type VARCHAR(50) NOT NULL,
    vcpu_reserved INTEGER,
    memory_mb_reserved INTEGER,
    gpu_reserved INTEGER,
    status VARCHAR(50) NOT NULL,
    starts_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES infrastructure_projects(id),
    FOREIGN KEY (agent_id) REFERENCES agent_registry(id),
    FOREIGN KEY (instance_id) REFERENCES infrastructure_instances(id),
    CONSTRAINT reservation_nonnegative CHECK ((vcpu_reserved IS NULL OR vcpu_reserved >= 0) AND (memory_mb_reserved IS NULL OR memory_mb_reserved >= 0) AND (gpu_reserved IS NULL OR gpu_reserved >= 0))
);

CREATE TABLE resource_usage_samples (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL,
    cpu_percent NUMERIC(7,3),
    memory_used_mb BIGINT,
    disk_used_gb NUMERIC(18,3),
    network_rx_bytes BIGINT,
    network_tx_bytes BIGINT,
    gpu_percent NUMERIC(7,3),
    metrics JSONB,
    FOREIGN KEY (instance_id) REFERENCES infrastructure_instances(id)
);

CREATE TABLE infrastructure_reconciliations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL,
    desired_state VARCHAR(50),
    observed_state VARCHAR(50),
    drift_detected BOOLEAN,
    drift_details JSONB,
    status VARCHAR(50),
    checked_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    FOREIGN KEY (instance_id) REFERENCES infrastructure_instances(id)
);

CREATE TABLE infrastructure_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID,
    operation_id UUID,
    instance_id UUID,
    actor_reference VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_reference VARCHAR(255),
    outcome VARCHAR(50),
    details JSONB,
    occurred_at TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (project_id) REFERENCES infrastructure_projects(id),
    FOREIGN KEY (operation_id) REFERENCES provisioning_operations(id),
    FOREIGN KEY (instance_id) REFERENCES infrastructure_instances(id)
);

CREATE INDEX idx_provider_accounts_provider ON provider_accounts(provider_id);
CREATE INDEX idx_provider_regions_provider ON provider_regions(provider_id);
CREATE INDEX idx_compute_profiles_provider ON compute_profiles(provider_id);
CREATE INDEX idx_instances_project_status ON infrastructure_instances(project_id, status);
CREATE INDEX idx_instances_account ON infrastructure_instances(provider_account_id);
CREATE INDEX idx_instances_provider_status ON infrastructure_instances(provider, status);
CREATE INDEX idx_instances_created_at ON infrastructure_instances(created_at);
CREATE INDEX idx_instances_metadata_gin ON infrastructure_instances USING GIN(metadata);
CREATE INDEX idx_instances_tags_gin ON infrastructure_instances USING GIN(tags);
CREATE INDEX idx_templates_project ON provisioning_templates(project_id);
CREATE INDEX idx_operations_instance ON provisioning_operations(instance_id);
CREATE INDEX idx_operations_project_status ON provisioning_operations(project_id, status);
CREATE INDEX idx_operations_created ON provisioning_operations(created_at);
CREATE INDEX idx_operations_correlation ON provisioning_operations(correlation_id);
CREATE INDEX idx_operations_retry ON provisioning_operations(next_retry_at) WHERE status = 'pending' AND next_retry_at IS NOT NULL;
CREATE INDEX idx_events_operation_created ON operation_events(operation_id, created_at);
CREATE INDEX idx_attempts_operation ON operation_attempts(operation_id);
CREATE INDEX idx_agent_assignments_agent ON agent_instance_assignments(agent_id);
CREATE INDEX idx_agent_assignments_instance ON agent_instance_assignments(instance_id);
CREATE INDEX idx_networks_project ON network_resources(project_id);
CREATE INDEX idx_network_attachments_instance ON instance_network_attachments(instance_id);
CREATE INDEX idx_network_attachments_network ON instance_network_attachments(network_id);
CREATE INDEX idx_volumes_project ON storage_volumes(project_id);
CREATE INDEX idx_volume_attachments_instance ON volume_attachments(instance_id);
CREATE INDEX idx_reservations_project_status ON resource_reservations(project_id, status);
CREATE INDEX idx_reservations_agent ON resource_reservations(agent_id);
CREATE INDEX idx_usage_instance_time ON resource_usage_samples(instance_id, collected_at DESC);
CREATE INDEX idx_reconciliations_instance_time ON infrastructure_reconciliations(instance_id, checked_at DESC);
CREATE INDEX idx_audit_occurred ON infrastructure_audit_log(occurred_at DESC);
CREATE INDEX idx_audit_instance ON infrastructure_audit_log(instance_id);
CREATE INDEX idx_audit_operation ON infrastructure_audit_log(operation_id);
