// SPDX-License-Identifier: Apache-2.0

package publish

// APIKeyRequirement is an APIKeyDef as data: what a sidecar's describe carries
// so a catalog shows the keys an item asks for before it is installed.
type APIKeyRequirement struct {
	EnvName     string `json:"env_name"`
	Label       string `json:"label,omitempty"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// Requirement is the key declaration as data.
func (k APIKeyDef) Requirement() APIKeyRequirement {
	return APIKeyRequirement{EnvName: k.EnvName, Label: k.Label, Required: k.Required, Description: k.Description}
}

// AuthRequirement is an AuthDef as data (its check, a function, stays behind):
// the login a sidecar relies on and how to perform it.
type AuthRequirement struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	LoginHint   string `json:"login_hint,omitempty"`
}

// Requirement is the login declaration as data.
func (a AuthDef) Requirement() AuthRequirement {
	return AuthRequirement{Name: a.Name, Label: a.Label, Description: a.Description, LoginHint: a.LoginHint}
}
