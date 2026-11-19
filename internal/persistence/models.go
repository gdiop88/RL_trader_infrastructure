package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

type InstanceStatus string

const (
	InstancePending      InstanceStatus = "pending"
	InstanceProvisioning InstanceStatus = "provisioning"
	InstanceRunning      InstanceStatus = "running"
	InstanceStopped      InstanceStatus = "stopped"
	InstanceFailed       InstanceStatus = "failed"
	InstanceDeleting     InstanceStatus = "deleting"
	InstanceDeleted      InstanceStatus = "deleted"
)

func (s InstanceStatus) Valid() bool {
	switch s {
	case InstancePending,
		InstanceProvisioning,
		InstanceRunning,
		InstanceStopped,
		InstanceFailed,
		InstanceDeleting,
		InstanceDeleted:
		return true
	default:
		return false
	}
}

type InfrastructureInstance struct {
	ID                 string          `json:"id"`
	Provider           string          `json:"provider"`
	ProviderInstanceID *string         `json:"provider_instance_id,omitempty"`
	Name               string          `json:"name"`
	Status             InstanceStatus  `json:"status"`
	Region             *string         `json:"region,omitempty"`
	InstanceType       *string         `json:"instance_type,omitempty"`
	PublicIP           *string         `json:"public_ip,omitempty"`
	PrivateIP          *string         `json:"private_ip,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	DeletedAt          *time.Time      `json:"deleted_at,omitempty"`
	Metadata           json.RawMessage `json:"metadata"`
	Tags               json.RawMessage `json:"tags"`
	LastError          *string         `json:"last_error,omitempty"`
	ProvisionedAt      *time.Time      `json:"provisioned_at,omitempty"`
	TerminatedAt       *time.Time      `json:"terminated_at,omitempty"`
	ProviderMetadataID *string         `json:"provider_metadata_id,omitempty"`
	ProjectID          *string         `json:"project_id,omitempty"`
	ProviderAccountID  *string         `json:"provider_account_id,omitempty"`
	ComputeProfileID   *string         `json:"compute_profile_id,omitempty"`
	DesiredState       *string         `json:"desired_state,omitempty"`
	LastObservedAt     *time.Time      `json:"last_observed_at,omitempty"`
}

type CreateInstanceInput struct {
	ID                 string
	Provider           string
	ProviderInstanceID *string
	Name               string
	Region             *string
	InstanceType       *string
	PublicIP           *string
	PrivateIP          *string
	Metadata           json.RawMessage
	Tags               json.RawMessage
	ProviderMetadataID *string
	ProjectID          *string
	ProviderAccountID  *string
	ComputeProfileID   *string
	DesiredState       *string
}

type ListInstancesOptions struct {
	Provider string
	Status   InstanceStatus
	Limit    int
	Offset   int
}

type UpdateProviderDetailsInput struct {
	ProviderInstanceID *string
	ProviderMetadataID *string
	Region             *string
	InstanceType       *string
	PublicIP           *string
	PrivateIP          *string
	Metadata           json.RawMessage
	Tags               json.RawMessage
}

type UpdateInstanceMetadataInput struct {
	Metadata json.RawMessage
	Tags     json.RawMessage
}

func validateOptionalString(
	name string,
	value *string,
	maxLength int,
) error {
	if value == nil {
		return nil
	}
	if len(*value) > maxLength {
		return fmt.Errorf("%s exceeds %d characters", name, maxLength)
	}
	return nil
}

func validateOptionalUUID(name string, value *string) error {
	if value == nil {
		return nil
	}
	if !validUUID(*value) {
		return fmt.Errorf("%s must be a valid UUID", name)
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, ch := range value {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			if !((ch >= '0' && ch <= '9') ||
				(ch >= 'a' && ch <= 'f') ||
				(ch >= 'A' && ch <= 'F')) {
				return false
			}
		}
	}
	return true
}

func validateIP(name string, value *string) error {
	if value == nil {
		return nil
	}
	if _, err := netip.ParseAddr(*value); err != nil {
		return fmt.Errorf("%s must be a valid IP address: %w", name, err)
	}
	return nil
}

func validateJSONType(
	name string,
	data json.RawMessage,
	expected string,
) error {
	if len(data) == 0 {
		return nil
	}
	if !json.Valid(data) {
		return fmt.Errorf("%s must contain valid JSON", name)
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	switch expected {
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s must be a JSON object", name)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("%s must be a JSON array", name)
		}
	}
	return nil
}

func (input CreateInstanceInput) Validate() error {
	if strings.TrimSpace(input.Provider) == "" {
		return errors.New("provider is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("instance name is required")
	}
	if len(input.Provider) > 50 {
		return errors.New("provider exceeds 50 characters")
	}
	if len(input.Name) > 255 {
		return errors.New("instance name exceeds 255 characters")
	}

	if input.ID != "" && !validUUID(input.ID) {
		return errors.New("instance ID must be a valid UUID")
	}

	fields := []struct {
		name  string
		value *string
		max   int
	}{
		{"provider instance ID", input.ProviderInstanceID, 255},
		{"region", input.Region, 100},
		{"instance type", input.InstanceType, 100},
		{"provider metadata ID", input.ProviderMetadataID, 255},
		{"desired state", input.DesiredState, 50},
	}

	for _, field := range fields {
		if err := validateOptionalString(
			field.name, field.value, field.max,
		); err != nil {
			return err
		}
	}

	for _, field := range []struct {
		name  string
		value *string
	}{
		{"project ID", input.ProjectID},
		{"provider account ID", input.ProviderAccountID},
		{"compute profile ID", input.ComputeProfileID},
	} {
		if err := validateOptionalUUID(
			field.name, field.value,
		); err != nil {
			return err
		}
	}

	if err := validateIP("public IP", input.PublicIP); err != nil {
		return err
	}
	if err := validateIP("private IP", input.PrivateIP); err != nil {
		return err
	}
	if err := validateJSONType(
		"metadata", input.Metadata, "object",
	); err != nil {
		return err
	}
	return validateJSONType("tags", input.Tags, "array")
}

func (input UpdateProviderDetailsInput) Validate() error {
	fields := []struct {
		name  string
		value *string
		max   int
	}{
		{"provider instance ID", input.ProviderInstanceID, 255},
		{"provider metadata ID", input.ProviderMetadataID, 255},
		{"region", input.Region, 100},
		{"instance type", input.InstanceType, 100},
	}

	for _, field := range fields {
		if err := validateOptionalString(
			field.name, field.value, field.max,
		); err != nil {
			return err
		}
	}

	if err := validateIP("public IP", input.PublicIP); err != nil {
		return err
	}
	if err := validateIP("private IP", input.PrivateIP); err != nil {
		return err
	}
	if err := validateJSONType(
		"metadata", input.Metadata, "object",
	); err != nil {
		return err
	}
	return validateJSONType("tags", input.Tags, "array")
}

func (input UpdateInstanceMetadataInput) Validate() error {
	if len(input.Metadata) == 0 && len(input.Tags) == 0 {
		return errors.New("metadata or tags must be provided")
	}
	if err := validateJSONType(
		"metadata", input.Metadata, "object",
	); err != nil {
		return err
	}
	return validateJSONType("tags", input.Tags, "array")
}
