// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"
)

// jobService implements syncapi.SyncJobService.
type jobService struct {
	cfg        *SyncServiceConfig
	jobs       *JobLibrary
	workspaces *WorkspaceLibrary
}

func (s *jobService) List(_ context.Context) (syncapi.ListJobsOutput, error) {
	return s.jobs.List()
}

func (s *jobService) Create(_ context.Context, in syncapi.CreateJobInput) (syncapi.CreateJobOutput, error) {
	return s.jobs.Create(in)
}

func (s *jobService) Get(_ context.Context, in syncapi.GetJobInput) (syncapi.GetJobOutput, error) {
	return s.jobs.Get(in.Name)
}

func (s *jobService) Update(_ context.Context, in syncapi.UpdateJobInput) (syncapi.UpdateJobOutput, error) {
	return s.jobs.Update(in)
}

func (s *jobService) Delete(_ context.Context, in syncapi.DeleteJobInput) (syncapi.DeleteJobOutput, error) {
	return s.jobs.Delete(in.Name)
}

func (s *jobService) Run(ctx context.Context, in syncapi.RunInput) (syncapi.RunOutput, error) {
	if in.All {
		return s.runAll(ctx, in.DryRun)
	}
	if in.Name == "" {
		return syncapi.RunOutput{}, fmt.Errorf("specify --name or --all")
	}
	result, err := s.runOne(ctx, in.Name, in.DryRun)
	if err != nil {
		return syncapi.RunOutput{}, err
	}
	return syncapi.RunOutput{Jobs: []syncapi.JobRunResult{result}}, nil
}

func (s *jobService) RunFromFile(ctx context.Context, in syncapi.RunFromFileInput) (syncapi.RunOutput, error) {
	if in.JobDef == "" {
		return syncapi.RunOutput{}, fmt.Errorf("jobdef is required")
	}
	if in.Dest == "" {
		return syncapi.RunOutput{}, fmt.Errorf("dest is required")
	}
	result, err := runFromFile(ctx, in.JobDef, in.Dest, s.cfg.Defaults, in.DryRun)
	if err != nil {
		return syncapi.RunOutput{}, err
	}
	return syncapi.RunOutput{Jobs: []syncapi.JobRunResult{result}}, nil
}

func (s *jobService) Status(ctx context.Context, in syncapi.StatusInput) (syncapi.StatusOutput, error) {
	job, err := s.jobs.LoadConfig(in.Name)
	if err != nil {
		return syncapi.StatusOutput{}, err
	}
	srcCfg, err := s.workspaces.LoadConfig(job.Source)
	if err != nil {
		return syncapi.StatusOutput{}, err
	}
	dstCfg, err := s.workspaces.LoadConfig(job.Dest)
	if err != nil {
		return syncapi.StatusOutput{}, err
	}
	changes, _ := DiffJob(ctx, job, srcCfg, dstCfg)
	return syncapi.StatusOutput{Name: in.Name, Changes: changes}, nil
}

func (s *jobService) runAll(ctx context.Context, dryRun bool) (syncapi.RunOutput, error) {
	jobList, err := s.jobs.List()
	if err != nil {
		return syncapi.RunOutput{}, err
	}
	var results []syncapi.JobRunResult
	for _, info := range jobList.Jobs {
		if !info.Enabled {
			results = append(results, syncapi.JobRunResult{Name: info.Name, Status: "skipped"})
			continue
		}
		result, err := s.runOne(ctx, info.Name, dryRun)
		if err != nil {
			results = append(results, syncapi.JobRunResult{Name: info.Name, Status: "error", Error: err.Error()})
			continue
		}
		result.Name = info.Name
		results = append(results, result)
	}
	return syncapi.RunOutput{Jobs: results}, nil
}

func (s *jobService) runOne(ctx context.Context, name string, dryRun bool) (syncapi.JobRunResult, error) {
	job, err := s.jobs.LoadConfig(name)
	if err != nil {
		return syncapi.JobRunResult{}, fmt.Errorf("load job %q: %w", name, err)
	}
	srcCfg, err := s.workspaces.LoadConfig(job.Source)
	if err != nil {
		return syncapi.JobRunResult{}, fmt.Errorf("load source workspace %q: %w", job.Source, err)
	}
	dstCfg, err := s.workspaces.LoadConfig(job.Dest)
	if err != nil {
		return syncapi.JobRunResult{}, fmt.Errorf("load dest workspace %q: %w", job.Dest, err)
	}
	if dryRun || job.Options.DryRun {
		dryRun = true
	}
	result := RunJob(ctx, job, srcCfg, dstCfg, s.cfg.Defaults, dryRun)
	result.Name = name
	return result, nil
}
