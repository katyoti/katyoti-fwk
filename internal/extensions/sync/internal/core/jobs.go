// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"
)

// JobConfig is the on-disk representation of a sync job.
type JobConfig struct {
	Source   string   `yaml:"source"`
	Dest     string   `yaml:"dest"`
	Mode     string   `yaml:"mode"` // push, pull, mirror, bisync
	Enabled  bool     `yaml:"enabled"`
	Schedule string   `yaml:"schedule,omitempty"`
	Watch    bool     `yaml:"watch,omitempty"`
	Filters  []string `yaml:"filters,omitempty"`
	Exclude  []string `yaml:"exclude,omitempty"`
	Options  JobOpts  `yaml:"options,omitempty"`
}

// JobOpts holds per-job sync options.
type JobOpts struct {
	Conflict  string `yaml:"conflict,omitempty"`
	Transfers int    `yaml:"transfers,omitempty"`
	Bandwidth string `yaml:"bandwidth,omitempty"`
	DryRun    bool   `yaml:"dry_run,omitempty"`
}

// JobLibrary manages job YAML files via Library[T].
type JobLibrary struct {
	lib *application.Library[JobConfig]
}

// JobLibraryExtension registers JobLibrary as a DI service.
var JobLibraryExtension = application.DeclareSimpleService[*JobLibrary](
	func(ctx context.Context) (*JobLibrary, error) {
		env, err := application.UseService[application.AppEnvironment](ctx)
		if err != nil {
			return nil, fmt.Errorf("JobLibrary: resolve AppEnvironment: %w", err)
		}
		dir := env.GetHomeSubDir("sync")
		ldr := loader.NewFSLoader(dir)
		return &JobLibrary{
			lib: application.NewLibrary[JobConfig]("jobs", ldr),
		}, nil
	},
)

func (jl *JobLibrary) Create(in syncapi.CreateJobInput) (syncapi.CreateJobOutput, error) {
	if jl.lib.Exists(in.Name) {
		return syncapi.CreateJobOutput{}, fmt.Errorf("job %q already exists", in.Name)
	}
	switch in.Mode {
	case "push", "pull", "mirror", "bisync":
	default:
		return syncapi.CreateJobOutput{}, fmt.Errorf("invalid mode %q (must be push, pull, mirror, or bisync)", in.Mode)
	}

	job := JobConfig{Source: in.Source, Dest: in.Dest, Mode: in.Mode, Enabled: true}
	header := fmt.Sprintf("# Sync Job: %s\n# %s → %s (%s)\n\n", in.Name, in.Source, in.Dest, in.Mode)
	if err := jl.lib.SaveWithHeader(in.Name, job, header); err != nil {
		return syncapi.CreateJobOutput{}, err
	}
	return syncapi.CreateJobOutput{Name: in.Name}, nil
}

func (jl *JobLibrary) List() (syncapi.ListJobsOutput, error) {
	var jobs []syncapi.JobInfo
	for _, name := range jl.lib.List() {
		job, err := jl.lib.Load(name)
		if err != nil || job == nil {
			continue
		}
		jobs = append(jobs, syncapi.JobInfo{
			Name: name, Source: job.Source, Dest: job.Dest,
			Mode: job.Mode, Enabled: job.Enabled, Schedule: job.Schedule,
		})
	}
	return syncapi.ListJobsOutput{Jobs: jobs}, nil
}

func (jl *JobLibrary) Get(name string) (syncapi.GetJobOutput, error) {
	m, err := jl.lib.LoadMap(name)
	if err != nil {
		return syncapi.GetJobOutput{}, fmt.Errorf("job %q: %w", name, err)
	}
	if m == nil {
		return syncapi.GetJobOutput{}, fmt.Errorf("job %q not found", name)
	}
	return syncapi.GetJobOutput{Name: name, Config: m}, nil
}

func (jl *JobLibrary) Update(in syncapi.UpdateJobInput) (syncapi.UpdateJobOutput, error) {
	job, err := jl.lib.Load(in.Name)
	if err != nil || job == nil {
		return syncapi.UpdateJobOutput{}, fmt.Errorf("job %q not found", in.Name)
	}
	if in.Enabled != nil {
		job.Enabled = *in.Enabled
	}
	if in.Mode != "" {
		job.Mode = in.Mode
	}
	if err := jl.lib.Save(in.Name, *job); err != nil {
		return syncapi.UpdateJobOutput{}, err
	}
	return syncapi.UpdateJobOutput{Name: in.Name, Updated: true}, nil
}

func (jl *JobLibrary) Delete(name string) (syncapi.DeleteJobOutput, error) {
	if !jl.lib.Exists(name) {
		return syncapi.DeleteJobOutput{Name: name}, fmt.Errorf("job %q not found", name)
	}
	if err := jl.lib.Delete(name); err != nil {
		return syncapi.DeleteJobOutput{Name: name}, err
	}
	return syncapi.DeleteJobOutput{Name: name, Deleted: true}, nil
}

func (jl *JobLibrary) LoadConfig(name string) (*JobConfig, error) {
	return jl.lib.Load(name)
}
