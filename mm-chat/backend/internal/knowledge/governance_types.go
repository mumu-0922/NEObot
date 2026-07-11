package knowledge

import (
	"context"
	"time"
)

type GovernanceManifest struct {
	Processor        string   `json:"processor"`
	EndpointID       string   `json:"endpointId"`
	ModelAPIVersion  string   `json:"modelApiVersion"`
	AllowedPurposes  []string `json:"allowedPurposes"`
	AllowedDataTypes []string `json:"allowedDataTypes"`
	Region           string   `json:"region"`
	RetentionPolicy  string   `json:"retentionPolicy"`
	DeletionContract string   `json:"deletionContract"`
	TrainingUse      string   `json:"trainingUse"`
}

type ProcessorGovernanceHead struct {
	Processor                string
	EndpointID               string
	Status                   string
	ActiveProfileID          string
	ActiveGovernanceRevision int64
	HeadRevision             int64
	ManifestHash             string
	UpdatedAt                time.Time
}

type GovernanceRepository interface {
	ApplyGovernance(context.Context, GovernanceManifest, string) (ProcessorGovernanceHead, error)
	DisableGovernance(context.Context, string, string) (ProcessorGovernanceHead, error)
}

var ErrGovernanceHeadNotFound = ValidationError{
	Code: "GOVERNANCE_HEAD_NOT_FOUND", Message: "processor governance head not found",
}
