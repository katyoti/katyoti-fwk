// SPDX-License-Identifier: Apache-2.0

package core

import api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"

func apiSkill(sidecar, name string) api.CreateSkillInput {
	return api.CreateSkillInput{Sidecar: sidecar, Name: name}
}

func apiTask(sidecar, name string) api.CreateTaskInput {
	return api.CreateTaskInput{Sidecar: sidecar, Name: name}
}

func apiPersona(sidecar, name string) api.CreatePersonaInput {
	return api.CreatePersonaInput{Sidecar: sidecar, Name: name}
}

func apiRef(sidecar, path string) api.CreateReferenceInput {
	return api.CreateReferenceInput{Sidecar: sidecar, Path: path}
}

func apiSidecar(name, include string) api.CreateSidecarInput {
	return api.CreateSidecarInput{Name: name, Include: include}
}
