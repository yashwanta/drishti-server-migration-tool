// Package reports generates migration and rollback reports.
package reports

import (
	"encoding/json"
	"fmt"
	"time"
	"github.com/drishti/hypershift/internal/domain"
)

type MigrationReport struct {
	ReportID          string                  `json:"report_id"`
	Type              string                  `json:"type"`
	PlanID            string                  `json:"plan_id"`
	JobID             string                  `json:"job_id"`
	SourceVM          string                  `json:"source_vm"`
	TargetNode        string                  `json:"target_node"`
	TargetVMID        int                     `json:"target_vmid"`
	Operator          string                  `json:"operator"`
	Status            string                  `json:"status"`
	StartedAt         *time.Time              `json:"started_at,omitempty"`
	FinishedAt        *time.Time              `json:"finished_at,omitempty"`
	Steps             []ReportStep            `json:"steps"`
	PreflightChecks   []domain.PreflightCheck `json:"preflight_checks"`
	Warnings          []string                `json:"warnings"`
	RetentionDeadline *time.Time              `json:"retention_deadline,omitempty"`
	GeneratedAt       time.Time               `json:"generated_at"`
}

type ReportStep struct {
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Message    string     `json:"message"`
}

func NewMigrationReport(plan domain.Plan, job *domain.Job, steps []ReportStep, checks []domain.PreflightCheck, warnings []string, retention *time.Time) MigrationReport {
	vmid := 0
	if plan.TargetVMID != nil { vmid = *plan.TargetVMID }
	return MigrationReport{
		ReportID:          "report-" + plan.ID,
		Type:              "migration",
		PlanID:            plan.ID,
		JobID:             job.ID,
		SourceVM:          plan.SourceVMID,
		TargetNode:        plan.TargetNodeID,
		TargetVMID:        vmid,
		Operator:          plan.CreatedBy,
		Status:            string(job.State),
		StartedAt:         job.StartedAt,
		FinishedAt:        job.FinishedAt,
		Steps:             steps,
		PreflightChecks:   checks,
		Warnings:          warnings,
		RetentionDeadline: retention,
		GeneratedAt:       time.Now().UTC(),
	}
}

func (r MigrationReport) ToJSON() (string, error) {
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil { return "", fmt.Errorf("marshal report: %w", err) }
	return string(out), nil
}